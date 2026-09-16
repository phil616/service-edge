package service

import (
	"fmt"
	"net"
	"strings"

	"github.com/dreamreflex/service-edge/internal/model"
)

// Direct frpc control transports. tcp / websocket multiplex over the
// frps TCP bind_port; kcp / quic each need a dedicated UDP port enabled on frps.
const (
	ProtoTCP       = "tcp"
	ProtoKCP       = "kcp"
	ProtoQUIC      = "quic"
	ProtoWebsocket = "websocket"
	ProtoWSS       = "wss"
)

// validProtocols is the set of accepted transport.protocol values.
var validProtocols = map[string]bool{
	ProtoTCP: true, ProtoKCP: true, ProtoQUIC: true, ProtoWebsocket: true, ProtoWSS: true,
}

// normalizeProtocol returns a valid protocol, defaulting empty to tcp.
func normalizeProtocol(p string) (string, error) {
	if p == "" {
		return ProtoTCP, nil
	}
	if !validProtocols[p] {
		return "", fmt.Errorf("%w: unknown transport protocol %q", ErrConflict, p)
	}
	return p, nil
}

// nodeOffersProtocol reports whether the node exposes the given transport.
// tcp/websocket ride bind_port; WSS requires a separately configured TLS gateway.
func nodeOffersProtocol(node model.FRPSNode, protocol string) bool {
	switch protocol {
	case ProtoTCP, ProtoWebsocket:
		return true
	case ProtoKCP:
		return node.KCPBindPort != nil
	case ProtoQUIC:
		return node.QUICBindPort != nil
	default:
		return false
	}
}

// validateClientProtocol checks the protocol is valid and offered by the node.
func validateClientProtocol(node model.FRPSNode, protocol string) (string, error) {
	p, err := normalizeProtocol(protocol)
	if err != nil {
		return "", err
	}
	if p == ProtoWSS {
		return "", fmt.Errorf("%w: 当前直连架构不支持 WSS；需要独立 TLS WebSocket 网关，请改用 TCP 或 WebSocket（均启用双向 TLS）", ErrConflict)
	}
	if !nodeOffersProtocol(node, p) {
		return "", fmt.Errorf("%w: 目标节点未启用 %s 传输，请先在节点上启用对应端口", ErrConflict, p)
	}
	return p, nil
}

// serverPortFor returns the frpc serverPort to dial for a protocol on a node.
func serverPortFor(node model.FRPSNode, protocol string) int {
	switch protocol {
	case ProtoKCP:
		if node.KCPBindPort != nil {
			return *node.KCPBindPort
		}
	case ProtoQUIC:
		if node.QUICBindPort != nil {
			return *node.QUICBindPort
		}
	}
	return node.BindPort
}

// nodeReservedPorts returns the frps host ports service-edge itself binds:
// bind_port, dashboard_port and any enabled kcp/quic ports. Used so a proxy
// remote_port can't collide with the node's own listeners.
func nodeReservedPorts(node model.FRPSNode) map[int]bool {
	used := map[int]bool{node.BindPort: true}
	if node.VhostHTTPPort > 0 {
		used[node.VhostHTTPPort] = true
	}
	if node.VhostHTTPSPort > 0 {
		used[node.VhostHTTPSPort] = true
	}
	if node.DashboardPort != nil {
		used[*node.DashboardPort] = true
	}
	if node.KCPBindPort != nil {
		used[*node.KCPBindPort] = true
	}
	if node.QUICBindPort != nil {
		used[*node.QUICBindPort] = true
	}
	return used
}

// validateNodeTransportPorts checks kcp/quic port choices for a node. QUIC must
// not share the TCP bind_port; neither may collide with the dashboard port.
func validateNodeTransportPorts(bindPort int, dashboardPort, kcp, quic *int) error {
	if bindPort < 1 || bindPort > 65535 {
		return fmt.Errorf("%w: bind_port must be in 1..65535", ErrConflict)
	}
	for _, port := range []*int{dashboardPort, kcp, quic} {
		if port != nil && (*port < 1 || *port > 65535) {
			return fmt.Errorf("%w: port must be in 1..65535", ErrConflict)
		}
	}
	if dashboardPort != nil && *dashboardPort == bindPort {
		return fmt.Errorf("%w: dashboard port conflicts with bind_port", ErrConflict)
	}
	if quic != nil && *quic == bindPort {
		return fmt.Errorf("%w: QUIC 端口不能与服务端口 (%d) 相同", ErrConflict, bindPort)
	}
	if kcp != nil && quic != nil && *kcp == *quic {
		return fmt.Errorf("%w: KCP 与 QUIC 端口不能相同", ErrConflict)
	}
	if dashboardPort != nil {
		if kcp != nil && *kcp == *dashboardPort {
			return fmt.Errorf("%w: KCP 端口不能与 Dashboard 端口相同", ErrConflict)
		}
		if quic != nil && *quic == *dashboardPort {
			return fmt.Errorf("%w: QUIC 端口不能与 Dashboard 端口相同", ErrConflict)
		}
	}
	return nil
}

func validateVhostPorts(node model.FRPSNode) error {
	used := map[int]bool{node.BindPort: true}
	for _, p := range []*int{node.DashboardPort, node.KCPBindPort, node.QUICBindPort} {
		if p != nil {
			used[*p] = true
		}
	}
	for _, port := range []int{node.VhostHTTPPort, node.VhostHTTPSPort} {
		if port == 0 {
			continue
		}
		if port < 1 || port > 65535 || used[port] {
			return fmt.Errorf("%w: HTTP/HTTPS 入口端口必须在 1..65535 且不与节点端口冲突", ErrConflict)
		}
		used[port] = true
	}
	return nil
}

func validateProxyListener(p ProxyMappingInput, node model.FRPSNode) error {
	if p.ProxyType != "http" && p.ProxyType != "https" {
		return nil
	}
	if (p.ProxyType == "http" && node.VhostHTTPPort == 0) || (p.ProxyType == "https" && node.VhostHTTPSPort == 0) {
		return fmt.Errorf("%w: 请先在 FRPS 节点配置 %s 入口端口", ErrConflict, p.ProxyType)
	}
	if p.Subdomain != "" && node.SubdomainHost == "" {
		return fmt.Errorf("%w: 请先在 FRPS 节点配置 subdomain_host", ErrConflict)
	}
	return nil
}

// A request's source address may be a reverse proxy or NAT gateway, not the
// public FRPS listener. Routing always uses an explicitly configured address.
func validateServerAddress(address string) error {
	if net.ParseIP(address) != nil {
		return nil
	}
	if address == "" || len(address) > 253 || strings.ContainsAny(address, "/: \t\r\n") {
		return fmt.Errorf("%w: public_ip 必须填写 FRPC 可达的 IP 或域名，不含协议和端口", ErrConflict)
	}
	for _, label := range strings.Split(strings.TrimSuffix(address, "."), ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("%w: invalid FRPS hostname", ErrConflict)
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
				return fmt.Errorf("%w: invalid FRPS hostname", ErrConflict)
			}
		}
	}
	return nil
}
