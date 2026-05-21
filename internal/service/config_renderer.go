package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dreamreflex/service-edge/internal/frp"
	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/protocol"
)

// RenderFRPSConfig renders frps.toml for the given node using the v0.61 flat-key
// TOML syntax (bindPort / auth.token / transport.tls.*). TLS is forced so frps
// only accepts TLS clients; cert/key paths point at the agent's config dir.
func RenderFRPSConfig(node *model.FRPSNode) string {
	p := frp.FRPSPaths()
	var b strings.Builder
	fmt.Fprintf(&b, "bindPort = %d\n", node.BindPort)
	// Enable the UDP-based control transports the node offers. KCP may reuse the
	// bindPort number; QUIC must be distinct (validated on write).
	if node.KCPBindPort != nil {
		fmt.Fprintf(&b, "kcpBindPort = %d\n", *node.KCPBindPort)
	}
	if node.QUICBindPort != nil {
		fmt.Fprintf(&b, "quicBindPort = %d\n", *node.QUICBindPort)
	}
	b.WriteString("\n")
	b.WriteString("auth.method = \"token\"\n")
	fmt.Fprintf(&b, "auth.token = %q\n\n", node.FrpToken)
	b.WriteString("transport.tls.force = true\n")
	fmt.Fprintf(&b, "transport.tls.certFile = %q\n", p.CertFile)
	fmt.Fprintf(&b, "transport.tls.keyFile = %q\n", p.KeyFile)
	// trustedCaFile enables frps->frpc verification (mutual TLS).
	fmt.Fprintf(&b, "transport.tls.trustedCaFile = %q\n\n", p.CAFile)

	if node.DashboardPort != nil && *node.DashboardPort > 0 {
		b.WriteString("webServer.addr = \"0.0.0.0\"\n")
		fmt.Fprintf(&b, "webServer.port = %d\n", *node.DashboardPort)
		fmt.Fprintf(&b, "webServer.user = %q\n", node.DashboardUser)
		fmt.Fprintf(&b, "webServer.password = %q\n\n", node.DashboardPwd)
	}

	fmt.Fprintf(&b, "log.to = %q\n", p.LogFile)
	b.WriteString("log.level = \"info\"\n")
	b.WriteString("log.maxDays = 7\n")
	return b.String()
}

// RenderFRPCConfig renders frpc.toml for the given client connecting to node.
// serverAddr is the public address frpc dials (node public IP or hostname).
// When adminPassword is non-empty, frpc's localhost admin API is enabled so the
// agent can read each proxy's real status (e.g. a remote_port that failed to bind).
func RenderFRPCConfig(conn *model.FRPCConnection, node *model.FRPSNode, serverAddr string, proxies []model.ProxyMapping, adminUser, adminPassword string) string {
	p := frp.FRPCPaths(conn.UUID)
	proto, _ := normalizeProtocol(conn.Protocol)
	var b strings.Builder
	fmt.Fprintf(&b, "serverAddr = %q\n", serverAddr)
	// serverPort depends on the transport: kcp/quic dial their UDP port, the rest
	// ride the TCP bindPort.
	fmt.Fprintf(&b, "serverPort = %d\n\n", serverPortFor(*node, proto))
	b.WriteString("auth.method = \"token\"\n")
	fmt.Fprintf(&b, "auth.token = %q\n\n", node.FrpToken)
	if proto != ProtoTCP {
		fmt.Fprintf(&b, "transport.protocol = %q\n", proto)
	}
	b.WriteString("transport.tls.enable = true\n")
	fmt.Fprintf(&b, "transport.tls.certFile = %q\n", p.CertFile)
	fmt.Fprintf(&b, "transport.tls.keyFile = %q\n", p.KeyFile)
	fmt.Fprintf(&b, "transport.tls.trustedCaFile = %q\n", p.CAFile)
	// Pin to the frps cert CN/SAN so hostname verification is independent of the
	// node's public IP (which may change or be unknown at creation time).
	fmt.Fprintf(&b, "transport.tls.serverName = %q\n\n", "frps-"+node.UUID)

	// Localhost-only admin API for proxy status introspection. Each connection on
	// a host uses a distinct admin port so multiple frpc processes don't collide.
	if adminPassword != "" {
		fmt.Fprintf(&b, "webServer.addr = %q\n", protocol.FRPCAdminAddr)
		fmt.Fprintf(&b, "webServer.port = %d\n", conn.AdminPort)
		fmt.Fprintf(&b, "webServer.user = %q\n", adminUser)
		fmt.Fprintf(&b, "webServer.password = %q\n\n", adminPassword)
	}

	fmt.Fprintf(&b, "log.to = %q\n", p.LogFile)
	b.WriteString("log.level = \"info\"\n")
	b.WriteString("log.maxDays = 7\n")

	for _, px := range proxies {
		// Inactive mappings (e.g. remote_port occupied on the host) are not
		// rendered, so frp never tries to bind a conflicting port.
		if px.Inactive {
			continue
		}
		b.WriteString("\n[[proxies]]\n")
		fmt.Fprintf(&b, "name = %q\n", px.Name)
		fmt.Fprintf(&b, "type = %q\n", px.ProxyType)
		localIP := px.LocalIP
		if localIP == "" {
			localIP = "127.0.0.1"
		}
		fmt.Fprintf(&b, "localIP = %q\n", localIP)
		fmt.Fprintf(&b, "localPort = %d\n", px.LocalPort)
		if px.RemotePort != nil && *px.RemotePort > 0 {
			fmt.Fprintf(&b, "remotePort = %d\n", *px.RemotePort)
		}
		if domains := parseDomains(px.CustomDomains); len(domains) > 0 {
			fmt.Fprintf(&b, "customDomains = %s\n", tomlStringArray(domains))
		}
		if px.Subdomain != "" {
			fmt.Fprintf(&b, "subdomain = %q\n", px.Subdomain)
		}
	}
	return b.String()
}

func parseDomains(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err == nil {
		return out
	}
	// Fall back to comma-separated.
	for _, part := range strings.Split(raw, ",") {
		if s := strings.TrimSpace(part); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func tomlStringArray(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
