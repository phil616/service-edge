//go:build integration

package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dreamreflex/service-edge/internal/frp"
	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/pki"
	"github.com/dreamreflex/service-edge/internal/protocol"
	"github.com/dreamreflex/service-edge/internal/service"
)

type localProcess struct {
	binary, config string
	cmd            *exec.Cmd
	done           chan struct{}
}
type localSupervisor struct {
	mu        sync.Mutex
	processes map[string]*localProcess
	root      string
}

func (s *localSupervisor) Configure(unit, binary, config string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.processes[unit]
	if p == nil {
		p = &localProcess{}
		s.processes[unit] = p
	}
	p.binary, p.config = binary, config
	return nil
}
func (s *localSupervisor) Enable(string) error { return nil }
func (s *localSupervisor) Stop(unit string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLocked(unit)
}
func (s *localSupervisor) stopLocked(unit string) error {
	p := s.processes[unit]
	if p != nil && p.cmd != nil {
		select {
		case <-p.done:
		default:
			_ = p.cmd.Process.Kill()
			<-p.done
		}
		p.cmd = nil
	}
	return nil
}
func (s *localSupervisor) Restart(unit string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.stopLocked(unit)
	p := s.processes[unit]
	if p == nil {
		return fmt.Errorf("unknown unit")
	}
	f, err := os.OpenFile(filepath.Join(s.root, unit+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	cmd := exec.Command(p.binary, "-c", p.config)
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Start(); err != nil {
		f.Close()
		return err
	}
	p.cmd = cmd
	p.done = make(chan struct{})
	done := p.done
	go func() { _ = cmd.Wait(); f.Close(); close(done) }()
	return nil
}
func (s *localSupervisor) WaitActive(unit string, timeout time.Duration) bool {
	s.mu.Lock()
	p := s.processes[unit]
	if p == nil || p.cmd == nil {
		s.mu.Unlock()
		return false
	}
	done := p.done
	s.mu.Unlock()
	select {
	case <-done:
		return false
	case <-time.After(time.Second):
		return true
	}
}
func (s *localSupervisor) pid(unit string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.processes[unit].cmd.Process.Pid
}

func testPaths(root, kind string) frp.DeployPaths {
	cfg := filepath.Join(root, "config")
	cert, key := "client.crt", "client.key"
	if kind == "frps" {
		cert, key = "server.crt", "server.key"
	}
	return frp.DeployPaths{ConfigDir: cfg, DataDir: filepath.Join(root, "data"), LogDir: filepath.Join(root, "logs"), ConfigFile: filepath.Join(cfg, kind+".toml"), CertFile: filepath.Join(cfg, cert), KeyFile: filepath.Join(cfg, key), CAFile: filepath.Join(cfg, "ca.crt"), LogFile: filepath.Join(root, "logs", kind+".log")}
}
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func waitHTTP(t *testing.T, port int, want string) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	client := http.Client{Timeout: time.Second}
	var last error
	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d", port))
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(b) == want {
				return
			}
			last = fmt.Errorf("response %q", b)
		} else {
			last = err
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("tunnel %d not ready: %v", port, last)
}

// Runs real frps/frpc, real generated configs, mTLS and HTTP payloads. The only
// substituted component is systemd, using an in-process child supervisor.
func TestRealFRPDeploymentAndRecovery(t *testing.T) {
	for _, transport := range []string{"tcp", "kcp", "quic", "websocket"} {
		t.Run(transport, func(t *testing.T) { testRealFRPDeploymentAndRecovery(t, transport) })
	}
}

func testRealFRPDeploymentAndRecovery(t *testing.T, transport string) {
	binDir := os.Getenv("FRP_TEST_BIN_DIR")
	if binDir == "" {
		t.Fatal("set FRP_TEST_BIN_DIR to extracted official FRP binaries")
	}
	root := t.TempDir()
	supervisor := &localSupervisor{root: root, processes: map[string]*localProcess{}}
	t.Cleanup(func() {
		for _, u := range []string{"a", "b", "server"} {
			_ = supervisor.Stop(u)
		}
	})
	t.Cleanup(func() {
		if t.Failed() {
			for _, u := range []string{"a", "b", "server"} {
				kind := "frpc"
				if u == "server" {
					kind = "frps"
				}
				b, _ := os.ReadFile(filepath.Join(root, u, "logs", kind+".log"))
				t.Logf("%s: %s", u, b)
			}
		}
	})
	caCert, caKey, err := pki.GenerateCAToString()
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "ca.crt"), []byte(caCert), 0600)
	os.WriteFile(filepath.Join(root, "ca.key"), []byte(caKey), 0600)
	ca, err := pki.LoadCA(filepath.Join(root, "ca.crt"), filepath.Join(root, "ca.key"))
	if err != nil {
		t.Fatal(err)
	}
	node := &model.FRPSNode{UUID: "server", BindPort: freePort(t), FrpToken: "integration-token"}
	kcp, quic := freeUDPPort(t), freeUDPPort(t)
	node.KCPBindPort, node.QUICBindPort = &kcp, &quic
	cert, err := ca.IssueServerCert(node.UUID, nil)
	if err != nil {
		t.Fatal(err)
	}
	sp := testPaths(filepath.Join(root, "server"), "frps")
	serverConfig := strings.NewReplacer(frp.FRPSBaseDir, filepath.Dir(sp.ConfigDir)).Replace(service.RenderFRPSConfig(node))
	serverBundle := &protocol.ConfigResponse{ConfigVersion: 1, FrpConfig: serverConfig, TLSCert: cert.CertPEM, TLSKey: cert.KeyPEM, CACert: caCert}
	server := &Applier{paths: sp, binary: filepath.Join(binDir, "frps"), unit: "server", systemd: supervisor}
	clientNode := *node
	var blackhole *atomic.Bool
	if transport == "tcp" {
		clientNode.BindPort, blackhole = startFaultRelay(t, node.BindPort)
	}
	clients := map[string]*Applier{}
	bundles := map[string]*protocol.ConfigResponse{}
	ports := map[string]int{}
	adminPorts := map[string]int{}
	for _, id := range []string{"a", "b"} {
		value := id
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, value) }))
		defer backend.Close()
		localPort := backend.Listener.Addr().(*net.TCPAddr).Port
		remote := freePort(t)
		ports[id] = remote
		conn := &model.FRPCConnection{UUID: id, AdminPort: freePort(t), Protocol: transport}
		adminPorts[id] = conn.AdminPort
		cert, err := ca.IssueClientCert(id)
		if err != nil {
			t.Fatal(err)
		}
		cp := testPaths(filepath.Join(root, id), "frpc")
		adminUser, adminPass := protocol.FRPCAdminCreds(id, "integration-api-token")
		config := service.RenderFRPCConfig(conn, &clientNode, "127.0.0.1", []model.ProxyMapping{{Name: "ssh", ProxyType: "tcp", LocalIP: "127.0.0.1", LocalPort: localPort, RemotePort: &remote}}, adminUser, adminPass)
		config = strings.NewReplacer(filepath.Dir(frp.FRPCPaths(id).ConfigDir), filepath.Dir(cp.ConfigDir)).Replace(config)
		bundle := &protocol.ConfigResponse{ConfigVersion: 1, FrpConfig: config, TLSCert: cert.CertPEM, TLSKey: cert.KeyPEM, CACert: caCert}
		applier := &Applier{paths: cp, binary: filepath.Join(binDir, "frpc"), unit: id, systemd: supervisor}
		if err := applier.Apply(bundle); err != nil {
			t.Fatalf("client starts before server: %v", err)
		}
		clients[id], bundles[id] = applier, bundle
	}
	beforeA, beforeB := supervisor.pid("a"), supervisor.pid("b")
	if err := server.Apply(serverBundle); err != nil {
		t.Fatal(err)
	}
	waitHTTP(t, ports["a"], "a")
	waitHTTP(t, ports["b"], "b")
	observer := &Runner{cfg: &Config{APIToken: "integration-api-token"}}
	for _, id := range []string{"a", "b"} {
		statuses, err := observer.queryProxyStatusesFor(context.Background(), id, adminPorts[id])
		if err != nil || len(statuses) != 1 || statuses[0].Name != "ssh" || statuses[0].Status != "running" {
			t.Fatalf("real admin observation %s: %+v %v", id, statuses, err)
		}
	}
	t.Log("two connections using the same display name both forward traffic; real admin observations match")
	if err := clients["a"].Apply(bundles["a"]); err != nil {
		t.Fatal(err)
	}
	if beforeA != supervisor.pid("a") {
		t.Fatal("unchanged snapshot restarted frpc")
	}
	if blackhole != nil {
		blackhole.Store(true)
		time.Sleep(35 * time.Second)
		blackhole.Store(false)
		waitHTTP(t, ports["a"], "a")
		waitHTTP(t, ports["b"], "b")
		if beforeA != supervisor.pid("a") || beforeB != supervisor.pid("b") {
			t.Fatal("network blackhole restarted frpc")
		}
		t.Log("35-second bidirectional network blackhole recovered without process restart")
	}
	_ = supervisor.Stop("server")
	time.Sleep(time.Second)
	if err := supervisor.Restart("server"); err != nil {
		t.Fatal(err)
	}
	waitHTTP(t, ports["a"], "a")
	waitHTTP(t, ports["b"], "b")
	if beforeA != supervisor.pid("a") || beforeB != supervisor.pid("b") {
		t.Fatal("reconnect restarted frpc")
	}
	t.Log("frps interruption recovered without restarting either frpc")
	oldLink, _ := os.Readlink(filepath.Join(clients["a"].paths.ConfigDir, "current"))
	invalid := *bundles["a"]
	invalid.TLSKey = caKey
	if err := clients["a"].Apply(&invalid); err == nil {
		t.Fatal("mismatched certificate accepted")
	}
	invalid = *bundles["a"]
	invalid.FrpConfig += "\nthis is not valid TOML {{{"
	if err := clients["a"].Apply(&invalid); err == nil {
		t.Fatal("invalid configuration accepted")
	}
	badBinary := filepath.Join(root, "bad-frpc")
	os.WriteFile(badBinary, []byte("#!/bin/sh\ncase \"$1\" in --version) echo 99.0.0;; verify) exit 0;; *) exit 42;; esac\n"), 0755)
	clients["a"].binary = badBinary
	if err := clients["a"].Apply(bundles["a"]); err == nil {
		t.Fatal("crashing upgrade accepted")
	}
	if link, _ := os.Readlink(filepath.Join(clients["a"].paths.ConfigDir, "current")); link != oldLink {
		t.Fatal("failed deployment did not restore complete generation")
	}
	waitHTTP(t, ports["a"], "a")
	waitHTTP(t, ports["b"], "b")
	t.Log("invalid cert/config rejected; crashing binary rolled back with traffic restored")
	clients["a"].binary = filepath.Join(binDir, "frpc")
	pending, err := os.MkdirTemp(filepath.Join(clients["a"].paths.DataDir, "revisions"), "revision-interrupted-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pending, "previous"), []byte(oldLink), 0600); err != nil {
		t.Fatal(err)
	}
	if err := switchGeneration(filepath.Join(clients["a"].paths.ConfigDir, "current"), pending); err != nil {
		t.Fatal(err)
	}
	if err := clients["a"].Apply(bundles["a"]); err != nil {
		t.Fatalf("recover interrupted activation: %v", err)
	}
	waitHTTP(t, ports["a"], "a")
	t.Log("interrupted activation recovered to previous generation")
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.LocalAddr().(*net.UDPAddr).Port
	l.Close()
	return port
}

// This relay keeps TCP sockets open while discarding encrypted bytes. Unlike a
// server exit, it exercises application keepalives under a silent network fault.
func startFaultRelay(t *testing.T, targetPort int) (int, *atomic.Bool) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	blocked := &atomic.Bool{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	sockets := map[net.Conn]bool{}
	stopped := false
	track := func(c net.Conn) bool {
		mu.Lock()
		defer mu.Unlock()
		if stopped {
			c.Close()
			return false
		}
		sockets[c] = true
		return true
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			if !track(client) {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer client.Close()
				upstream, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", targetPort), time.Second)
				if err != nil {
					return
				}
				if !track(upstream) {
					return
				}
				defer upstream.Close()
				done := make(chan struct{})
				go func() { _, _ = io.Copy(faultWriter{upstream, blocked}, client); upstream.Close(); close(done) }()
				_, _ = io.Copy(faultWriter{client, blocked}, upstream)
				client.Close()
				<-done
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		mu.Lock()
		stopped = true
		for c := range sockets {
			c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return listener.Addr().(*net.TCPAddr).Port, blocked
}

type faultWriter struct {
	conn    net.Conn
	blocked *atomic.Bool
}

func (w faultWriter) Write(p []byte) (int, error) {
	if w.blocked.Load() {
		return len(p), nil
	}
	return w.conn.Write(p)
}
