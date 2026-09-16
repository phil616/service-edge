package service

import (
	"github.com/dreamreflex/service-edge/internal/model"
	"strings"
	"testing"
)

func TestVhostListenersAreRenderedAndReserved(t *testing.T) {
	node := model.FRPSNode{BindPort: 7000, VhostHTTPPort: 8080, VhostHTTPSPort: 8443, SubdomainHost: "edge.example.com"}
	if err := validateVhostPorts(node); err != nil {
		t.Fatal(err)
	}
	rendered := RenderFRPSConfig(&node)
	for _, value := range []string{"vhostHTTPPort = 8080", "vhostHTTPSPort = 8443", `subDomainHost = "edge.example.com"`} {
		if !strings.Contains(rendered, value) {
			t.Fatal("missing listener", value)
		}
	}
	for _, port := range []int{7000, 8080, 8443} {
		if !nodeReservedPorts(node)[port] {
			t.Fatal("unreserved", port)
		}
	}
	node.VhostHTTPSPort = 8080
	if err := validateVhostPorts(node); err == nil {
		t.Fatal("duplicate listener accepted")
	}
	for _, kind := range []string{"http", "https"} {
		if err := validateProxyListener(ProxyMappingInput{ProxyType: kind}, model.FRPSNode{}); err == nil {
			t.Fatal("unavailable vhost accepted")
		}
	}
}

func TestFRPSAddressMustBeExplicitAndUnambiguous(t *testing.T) {
	for _, address := range []string{"", "https://edge.example.com", "edge.example.com:7000", " edge.example.com", "-bad.example.com", "edge.example.com/path"} {
		if err := validateServerAddress(address); err == nil {
			t.Fatal("invalid address", address)
		}
	}
	for _, address := range []string{"edge.example.com", "127.0.0.1", "2001:db8::1"} {
		if err := validateServerAddress(address); err != nil {
			t.Fatal(address, err)
		}
	}
}

func TestChangingProxyTypeDropsIncompatibleFields(t *testing.T) {
	port := 18000
	input := ProxyMappingInput{Name: "proxy", ProxyType: "http", LocalPort: 80, RemotePort: &port, CustomDomains: []string{"example.com"}, Subdomain: "web"}
	web := input.toModel("conn")
	if web.RemotePort != nil || web.CustomDomains == "" {
		t.Fatal(web)
	}
	input.ProxyType = "tcp"
	tcp := input.toModel("conn")
	if tcp.RemotePort == nil || tcp.CustomDomains != "" || tcp.Subdomain != "" {
		t.Fatal(tcp)
	}
}
