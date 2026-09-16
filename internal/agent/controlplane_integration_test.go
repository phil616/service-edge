//go:build integration

package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dreamreflex/service-edge/internal/api"
	"github.com/dreamreflex/service-edge/internal/api/handler"
	"github.com/dreamreflex/service-edge/internal/api/middleware"
	"github.com/dreamreflex/service-edge/internal/config"
	"github.com/dreamreflex/service-edge/internal/frp"
	"github.com/dreamreflex/service-edge/internal/pki"
	"github.com/dreamreflex/service-edge/internal/protocol"
	"github.com/dreamreflex/service-edge/internal/service"
	"github.com/dreamreflex/service-edge/internal/store"
)

func (s *localSupervisor) Disable(string) error { return nil }
func (s *localSupervisor) ProcessStatus(ctx context.Context, unit string) (bool, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.processes[unit]
	if p == nil || p.cmd == nil {
		return false, 0, nil
	}
	select {
	case <-p.done:
		return false, 0, nil
	default:
		return true, p.cmd.Process.Pid, nil
	}
}
func (s *localSupervisor) IsActive(unit string) bool {
	active, _, _ := s.ProcessStatus(context.Background(), unit)
	return active
}
func (s *localSupervisor) MainPID(unit string) int {
	_, pid, _ := s.ProcessStatus(context.Background(), unit)
	return pid
}

// Exercise the real control-plane HTTP contract and host reconciler together;
// only the systemd adapter is substituted so this also runs in unprivileged CI.
func TestRealFRPControlPlaneLifecycle(t *testing.T) {
	binDir := os.Getenv("FRP_TEST_BIN_DIR")
	if binDir == "" {
		t.Fatal("set FRP_TEST_BIN_DIR")
	}
	root := t.TempDir()
	cert, key, err := pki.GenerateCAToString()
	if err != nil {
		t.Fatal(err)
	}
	certPath, keyPath := filepath.Join(root, "ca.crt"), filepath.Join(root, "ca.key")
	if err = os.WriteFile(certPath, []byte(cert), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(keyPath, []byte(key), 0600); err != nil {
		t.Fatal(err)
	}
	ca, err := pki.LoadCA(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(root, "db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{AgentAPIToken: "integration-master-secret", EnrollmentTokenTTL: config.Duration(time.Hour), FRPDistDir: filepath.Join(root, "dist")}
	svc := service.New(st, ca, cfg)
	router := api.NewRouter(api.Options{Cfg: cfg, Handler: &handler.Handler{Svc: svc}, JWT: middleware.NewJWTManager("jwt", time.Hour), FRPDistDir: cfg.FRPDistDir})
	server := httptest.NewServer(router)
	defer server.Close()
	cfg.Server.ExternalURL = server.URL
	version := frp.FrpVersion(filepath.Join(binDir, "frpc"))
	archive := filepath.Join(filepath.Dir(binDir), "frp_"+version+"_linux_amd64.tar.gz")
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = svc.UploadFRPDist(filepath.Base(archive), f); err != nil {
		t.Fatal(err)
	}
	node, err := svc.CreateFRPS(service.CreateFRPSInput{Name: "edge", BindPort: freePort(t), PublicIP: "127.0.0.1", FrpVersion: version, VhostHTTPPort: freePort(t), VhostHTTPSPort: freePort(t)})
	if err != nil {
		t.Fatal(err)
	}
	host, err := svc.CreateFRPCHost(service.CreateFRPCHostInput{Name: "host", FrpVersion: version})
	if err != nil {
		t.Fatal(err)
	}
	supervisor := &localSupervisor{root: root, processes: map[string]*localProcess{}}
	t.Cleanup(func() {
		for unit := range supervisor.processes {
			_ = supervisor.Stop(unit)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	enroll := func(kind, uuid string) *Client {
		client := NewClient(&Config{APIEndpoint: server.URL, APIToken: protocol.AgentToken(cfg.AgentAPIToken, kind, uuid), AgentType: kind, UUID: uuid})
		tok, err := svc.CreateEnrollment(kind, uuid)
		if err != nil {
			t.Fatal(err)
		}
		if err = client.Enroll(ctx, tok.Token, protocol.EnrollRequest{UUID: uuid, AgentType: kind}); err != nil {
			t.Fatal(err)
		}
		return client
	}
	sc := enroll("frps", node.UUID)
	sb, _, err := sc.PollConfig(ctx, 0, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	sp := testPaths(filepath.Join(root, "server"), "frps")
	sb.FrpConfig = strings.ReplaceAll(sb.FrpConfig, frp.FRPSBaseDir, filepath.Dir(sp.ConfigDir))
	sr := &Runner{cfg: &Config{AgentType: "frps", FrpBinaryPath: filepath.Join(root, "server-bin", "frps")}, client: sc, state: LoadState(filepath.Join(root, "server-state.json")), systemd: supervisor, unit: "server", applier: &Applier{paths: sp, unit: "server", systemd: supervisor}}
	if !sr.applyBundle(ctx, sb) {
		t.Fatal("server failed")
	}
	hc := enroll("frpc", host.UUID)
	hr := &Runner{cfg: &Config{AgentType: "frpc", APIToken: protocol.AgentToken(cfg.AgentAPIToken, "frpc", host.UUID), FrpBinaryPath: filepath.Join(root, "host-bin", "frpc")}, client: hc, state: LoadState(filepath.Join(root, "host-state.json")), systemd: supervisor, instanceBase: filepath.Join(root, "host")}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "through-control-plane") }))
	defer backend.Close()
	tlsBackend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "tls-through-control-plane") }))
	defer tlsBackend.Close()
	udpBackend, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udpBackend.Close()
	go func() {
		buf := make([]byte, 1024)
		for {
			n, addr, err := udpBackend.ReadFrom(buf)
			if err != nil {
				return
			}
			udpBackend.WriteTo(buf[:n], addr)
		}
	}()
	udpRemote := freeUDPPort(t)
	remote := freePort(t)
	conn, err := svc.CreateConnection(host.UUID, service.CreateConnectionInput{Name: "connection", FRPSUUID: node.UUID, Proxies: []service.ProxyMappingInput{{Name: "web", ProxyType: "tcp", LocalIP: "127.0.0.1", LocalPort: backend.Listener.Addr().(*net.TCPAddr).Port, RemotePort: &remote},
		{Name: "http", ProxyType: "http", LocalIP: "127.0.0.1", LocalPort: backend.Listener.Addr().(*net.TCPAddr).Port, CustomDomains: []string{"web.example.test"}},
		{Name: "https", ProxyType: "https", LocalIP: "127.0.0.1", LocalPort: tlsBackend.Listener.Addr().(*net.TCPAddr).Port, CustomDomains: []string{"secure.example.test"}},
		{Name: "udp", ProxyType: "udp", LocalIP: "127.0.0.1", LocalPort: udpBackend.LocalAddr().(*net.UDPAddr).Port, RemotePort: &udpRemote}}})
	if err != nil {
		t.Fatal(err)
	}
	// An ephemeral admin port keeps CI independent of other FRP instances.
	if err = st.DB.Model(conn).Update("admin_port", freePort(t)).Error; err != nil {
		t.Fatal(err)
	}
	hb, _, err := hc.PollHostConfig(ctx, 0, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if !hr.reconcile(ctx, hb) {
		t.Fatal("host failed")
	}
	waitHTTP(t, remote, "through-control-plane")
	// Verify domain routing and TLS passthrough with actual payloads.
	for _, tc := range []struct {
		scheme, host, want string
		port               int
	}{{"http", "web.example.test", "through-control-plane", node.VhostHTTPPort}, {"https", "secure.example.test", "tls-through-control-plane", node.VhostHTTPSPort}} {
		transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(c context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(c, network, fmt.Sprintf("127.0.0.1:%d", tc.port))
		}}
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		resp, err := client.Get(tc.scheme + "://" + tc.host)
		if err != nil {
			t.Fatal(tc.scheme, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		transport.CloseIdleConnections()
		if string(body) != tc.want {
			t.Fatal(tc.scheme, string(body))
		}
	}
	udp, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", udpRemote))
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	udp.SetDeadline(time.Now().Add(3 * time.Second))
	udp.Write([]byte("udp-tunnel"))
	packet := make([]byte, 64)
	n, err := udp.Read(packet)
	if err != nil || string(packet[:n]) != "udp-tunnel" {
		t.Fatal("UDP traffic", err)
	}
	t.Log("TCP, UDP, HTTP domain routing and HTTPS passthrough all forwarded actual payloads")
	hr.reportHostStatus(ctx)
	observed, err := svc.GetConnection(conn.UUID)
	if err != nil || observed.Status != "online" {
		t.Fatal(observed, err)
	}
	t.Log("enrollment -> authenticated poll -> archive download -> reconcile -> real traffic -> observed online")
	backend.Close()
	hr.reportHostStatus(ctx)
	observed, err = svc.GetConnection(conn.UUID)
	if err != nil || observed.Status != "degraded" || len(observed.Proxies) != 4 || observed.Proxies[0].LocalReachable == nil || *observed.Proxies[0].LocalReachable {
		t.Fatal("registered proxy hid failed backend", observed, err)
	}
	t.Log("backend stopped while Agent and frpc remain running -> connection degraded with local dial error")
	// Delete only the state file: the running instance must be found from disk.
	if err = os.Remove(hr.state.path); err != nil {
		t.Fatal(err)
	}
	hr.state = LoadState(hr.state.path)
	if err = svc.DeleteConnection(conn.UUID); err != nil {
		t.Fatal(err)
	}
	hb, _, err = hc.PollHostConfig(ctx, 0, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if !hr.reconcile(ctx, hb) || supervisor.IsActive(frpcUnit(conn.UUID)) {
		t.Fatal("orphan tunnel survived removal")
	}
	if err = svc.DeleteFRPCHost(host.UUID); err != nil {
		t.Fatal(err)
	}
	hb, _, err = hc.PollHostConfig(ctx, 0, "linux", "amd64")
	if err != nil || !hb.Decommission {
		t.Fatal(hb, err)
	}
	if !hr.decommission(ctx, hb.ConfigVersion) {
		t.Fatal("host retirement failed")
	}
	if err = svc.DeleteFRPS(node.UUID); err != nil {
		t.Fatal(err)
	}
	sb, _, err = sc.PollConfig(ctx, 0, "linux", "amd64")
	if err != nil || !sb.Decommission {
		t.Fatal(sb, err)
	}
	if !sr.decommission(ctx, sb.ConfigVersion) || supervisor.IsActive("server") {
		t.Fatal("server retirement failed")
	}
	for _, identity := range []struct{ kind, id string }{{"frpc", host.UUID}, {"frps", node.UUID}} {
		row, err := svc.AgentRetirement(identity.kind, identity.id)
		if err != nil || row.CompletedAt == nil {
			t.Fatal(row, err)
		}
	}
	t.Log("lost local state -> orphan stopped; host/server delete -> durable stop instruction -> confirmed shutdown")
}
