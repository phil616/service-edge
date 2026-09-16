package service

import (
	"github.com/dreamreflex/service-edge/internal/agent"
	"github.com/dreamreflex/service-edge/internal/config"
	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/protocol"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGeneratedInstallerMatchesAgentConfiguration(t *testing.T) {
	for _, kind := range []string{"frpc", "frps"} {
		t.Run(kind, func(t *testing.T) {
			s := newTestService(t)
			s.Cfg = &config.Config{AgentAPIToken: "token-with-$dollar-`literal`-'quote", AgentDownloadBase: "https://cdn.example.com/agent"}
			s.Cfg.Server.ExternalURL = "https://edge.example.com"
			if kind == "frpc" {
				if err := s.Store.DB.Create(&model.FRPCHost{UUID: "host", Name: "host", FrpVersion: "v0.61.1"}).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := s.Store.DB.Create(&model.FRPSNode{UUID: "host", Name: "host", FrpVersion: "v0.61.1"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Store.DB.Create(&model.EnrollmentToken{Token: "enrollment", TargetType: kind, TargetUUID: "host", ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
				t.Fatal(err)
			}
			script, err := s.RenderInstallScript(kind, "enrollment")
			if err != nil {
				t.Fatal(err)
			}
			verifyInstallerRollback(t, script, kind)
			cmd := exec.Command("bash", "-n")
			cmd.Stdin = strings.NewReader(script)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("invalid shell: %v: %s", err, out)
			}
			_, body, ok := strings.Cut(script, "<<'SERVICE_EDGE_AGENT_CONFIG'\n")
			if !ok {
				t.Fatal("missing generated Agent config")
			}
			body, _, ok = strings.Cut(body, "SERVICE_EDGE_AGENT_CONFIG\n")
			if !ok {
				t.Fatal("missing config terminator")
			}
			file := filepath.Join(t.TempDir(), "agent.yaml")
			if err := os.WriteFile(file, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := agent.LoadConfig(file)
			if err != nil {
				t.Fatalf("installer produces unusable Agent: %v", err)
			}
			if cfg.APIToken != protocol.AgentToken(s.Cfg.AgentAPIToken, kind, "host") || cfg.AgentType != kind || cfg.ConfigPollTimeout.Std() != 60*time.Second {
				t.Fatal("generated config lost values")
			}
			if strings.Contains(script, "/tmp/frp_") || strings.Contains(script, "/tmp/frp.tar.gz") {
				t.Fatal("shared temporary archive path")
			}
			if strings.Contains(script, "FRP_BASE_URL") {
				t.Fatal("Agent installation still depends on FRP download")
			}
		})
	}
}
func TestEnrollmentRetryIsBoundToIdentity(t *testing.T) {
	s := newTestService(t)
	if err := s.Store.DB.Create(&model.FRPCHost{UUID: "h", Name: "host"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Store.DB.Create(&model.EnrollmentToken{Token: "t", TargetType: "frpc", TargetUUID: "h", ExpiresAt: time.Now().Add(time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := s.ConsumeEnrollment("t", "h", "frpc"); err != nil {
			t.Fatal("lost-response retry failed", err)
		}
	}
	if _, err := s.ConsumeEnrollment("t", "other", "frpc"); err == nil {
		t.Fatal("token reused for another host")
	}
	if _, err := s.ConsumeEnrollment("t", "h", "frps"); err == nil {
		t.Fatal("token reused for another role")
	}
}

// Execute the rendered installer with isolated paths and fake network/systemd.
// The candidate passes validation but fails its final liveness check.
func verifyInstallerRollback(t *testing.T, script, kind string) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "commands")
	install := filepath.Join(root, "opt", kind+"-agent")
	units := filepath.Join(root, "units")
	for _, dir := range []string{bin, filepath.Join(install, "bin"), filepath.Join(install, "config"), units} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	previous := map[string]string{filepath.Join(install, "bin", "agent"): "old executable", filepath.Join(install, "config", "agent.yaml"): "uuid: host\n", filepath.Join(units, "service-edge-"+kind+"-agent.service"): "old unit"}
	for path, content := range previous {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	commands := map[string]string{
		"curl": `#!/bin/bash
while (($#)); do
 if [[ "$1" == -o ]]; then printf '#!/bin/sh\nexit 0\n' > "$2"; exit 0; fi
 shift
done
exit 0
`,
		"systemctl": "#!/bin/sh\ncase \"$1\" in is-active|is-enabled) exit 1;; *) exit 0;; esac\n",
		"sleep":     "#!/bin/sh\nexit 0\n",
	}
	for name, content := range commands {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	script = strings.NewReplacer("/opt/service-edge", filepath.Join(root, "opt"), "/etc/systemd/system", units, "[[ $EUID -eq 0 ]]", "true").Replace(script)
	cmd := exec.Command("bash")
	cmd.Stdin = strings.NewReader(script)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("failed installer reported success")
	}
	if !strings.Contains(string(out), "restored previous installation") {
		t.Fatalf("rollback did not run: %v %s", err, out)
	}
	for path, want := range previous {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("rollback lost %s: %q %v", path, got, err)
		}
	}
}
