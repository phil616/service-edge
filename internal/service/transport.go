package service

import (
	"fmt"

	"github.com/dreamreflex/service-edge/internal/model"
)

// Supported frpc control transports. tcp / websocket / wss multiplex over the
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
// tcp/websocket/wss always ride bind_port; kcp/quic need their UDP port enabled.
func nodeOffersProtocol(node model.FRPSNode, protocol string) bool {
	switch protocol {
	case ProtoTCP, ProtoWebsocket, ProtoWSS:
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
