package service

import (
	"strings"
	"testing"

	"github.com/dreamreflex/service-edge/internal/model"
)

func TestRenderFRPSConfig(t *testing.T) {
	port := 7500
	node := &model.FRPSNode{
		UUID:          "node-1",
		BindPort:      7000,
		FrpToken:      "secrettoken",
		DashboardPort: &port,
		DashboardUser: "admin",
		DashboardPwd:  "pw",
	}
	out := RenderFRPSConfig(node)
	for _, want := range []string{
		"bindPort = 7000",
		`auth.method = "token"`,
		`auth.token = "secrettoken"`,
		"transport.tls.force = true",
		"webServer.port = 7500",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("frps config missing %q\n%s", want, out)
		}
	}
}

func TestRenderFRPCConfigProxies(t *testing.T) {
	remote := 6022
	client := &model.FRPCConnection{UUID: "client-1", AdminPort: 7400}
	node := &model.FRPSNode{UUID: "node-1", BindPort: 7000, FrpToken: "tok"}
	proxies := []model.ProxyMapping{
		{Name: "ssh", ProxyType: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: &remote},
		{Name: "web", ProxyType: "http", LocalIP: "127.0.0.1", LocalPort: 8080, CustomDomains: `["a.example.com","b.example.com"]`},
	}
	out := RenderFRPCConfig(client, node, "203.0.113.10", proxies, "admin", "secretpw")
	for _, want := range []string{
		`serverAddr = "203.0.113.10"`,
		"serverPort = 7000",
		`transport.tls.serverName = "frps-node-1"`,
		`webServer.password = "secretpw"`,
		"[[proxies]]",
		`name = "ssh"`,
		"remotePort = 6022",
		`customDomains = ["a.example.com", "b.example.com"]`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("frpc config missing %q\n%s", want, out)
		}
	}
}

func TestRenderTransportProtocols(t *testing.T) {
	kcp, quic := 7000, 7001
	node := &model.FRPSNode{UUID: "node-1", BindPort: 7000, FrpToken: "tok", KCPBindPort: &kcp, QUICBindPort: &quic}

	frps := RenderFRPSConfig(node)
	for _, want := range []string{"bindPort = 7000", "kcpBindPort = 7000", "quicBindPort = 7001"} {
		if !strings.Contains(frps, want) {
			t.Errorf("frps config missing %q\n%s", want, frps)
		}
	}

	// kcp client dials the kcp UDP port; protocol line emitted.
	kcpClient := &model.FRPCConnection{UUID: "c-kcp", Protocol: "kcp"}
	out := RenderFRPCConfig(kcpClient, node, "203.0.113.10", nil, "", "")
	if !strings.Contains(out, `transport.protocol = "kcp"`) || !strings.Contains(out, "serverPort = 7000") {
		t.Errorf("kcp frpc config wrong\n%s", out)
	}

	// quic client dials the quic UDP port.
	quicClient := &model.FRPCConnection{UUID: "c-quic", Protocol: "quic"}
	out = RenderFRPCConfig(quicClient, node, "203.0.113.10", nil, "", "")
	if !strings.Contains(out, `transport.protocol = "quic"`) || !strings.Contains(out, "serverPort = 7001") {
		t.Errorf("quic frpc config wrong\n%s", out)
	}

	// tcp (default) dials bindPort and emits no protocol line.
	tcpClient := &model.FRPCConnection{UUID: "c-tcp", Protocol: "tcp"}
	out = RenderFRPCConfig(tcpClient, node, "203.0.113.10", nil, "", "")
	if strings.Contains(out, "transport.protocol") || !strings.Contains(out, "serverPort = 7000") {
		t.Errorf("tcp frpc config wrong\n%s", out)
	}
}

func TestRenderFRPCConfigSkipsInactive(t *testing.T) {
	active, conflicting := 6022, 6023
	client := &model.FRPCConnection{UUID: "client-1", AdminPort: 7400}
	node := &model.FRPSNode{UUID: "node-1", BindPort: 7000, FrpToken: "tok"}
	proxies := []model.ProxyMapping{
		{Name: "ssh", ProxyType: "tcp", LocalPort: 22, RemotePort: &active},
		{Name: "db", ProxyType: "tcp", LocalPort: 5432, RemotePort: &conflicting, Inactive: true},
	}
	out := RenderFRPCConfig(client, node, "203.0.113.10", proxies, "", "")
	if !strings.Contains(out, `name = "ssh"`) {
		t.Errorf("active proxy should be rendered\n%s", out)
	}
	if strings.Contains(out, `name = "db"`) || strings.Contains(out, "remotePort = 6023") {
		t.Errorf("inactive proxy must not be rendered\n%s", out)
	}
}
