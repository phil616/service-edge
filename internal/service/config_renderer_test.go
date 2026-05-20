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
	client := &model.FRPCClient{UUID: "client-1"}
	node := &model.FRPSNode{UUID: "node-1", BindPort: 7000, FrpToken: "tok"}
	proxies := []model.ProxyMapping{
		{Name: "ssh", ProxyType: "tcp", LocalIP: "127.0.0.1", LocalPort: 22, RemotePort: &remote},
		{Name: "web", ProxyType: "http", LocalIP: "127.0.0.1", LocalPort: 8080, CustomDomains: `["a.example.com","b.example.com"]`},
	}
	out := RenderFRPCConfig(client, node, "203.0.113.10", proxies)
	for _, want := range []string{
		`serverAddr = "203.0.113.10"`,
		"serverPort = 7000",
		`transport.tls.serverName = "frps-node-1"`,
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
