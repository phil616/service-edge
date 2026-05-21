package service

import (
	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/pki"
)

// CAInfo returns a summary of the control-plane CA certificate.
func (s *Service) CAInfo() *pki.CertInfo {
	return s.CA.Info()
}

// LeafCertInfo parses a stored leaf certificate PEM into a summary (nil on error).
func (s *Service) LeafCertInfo(certPEM string) *pki.CertInfo {
	if certPEM == "" {
		return nil
	}
	info, err := pki.ParseCertInfo(certPEM)
	if err != nil {
		return nil
	}
	return info
}

// Topology bundles every frps node with every frpc client (and its proxies) so
// the UI can render the full network/port relationship graph in one request.
type Topology struct {
	FRPS []model.FRPSNode   `json:"frps"`
	FRPC []model.FRPCClient `json:"frpc"`
}

func (s *Service) Topology() (*Topology, error) {
	nodes, err := s.ListFRPS()
	if err != nil {
		return nil, err
	}
	clients, err := s.ListFRPC()
	if err != nil {
		return nil, err
	}
	for i := range clients {
		proxies, err := s.ListProxies(clients[i].UUID)
		if err != nil {
			return nil, err
		}
		clients[i].Proxies = proxies
	}
	return &Topology{FRPS: nodes, FRPC: clients}, nil
}

// PortUse describes one occupied remote port on an frps node and what holds it.
type PortUse struct {
	Port      int    `json:"port"`
	Kind      string `json:"kind"` // bind | dashboard | proxy
	FRPCUUID  string `json:"frpc_uuid,omitempty"`
	FRPCName  string `json:"frpc_name,omitempty"`
	ProxyName string `json:"proxy_name,omitempty"`
	ProxyType string `json:"proxy_type,omitempty"`
}

// PortUsage returns the detailed list of occupied ports on an frps node,
// attributing each proxy port to its owning frpc client.
func (s *Service) PortUsage(frpsUUID string) ([]PortUse, error) {
	node, err := s.GetFRPS(frpsUUID)
	if err != nil {
		return nil, err
	}
	out := []PortUse{{Port: node.BindPort, Kind: "bind"}}
	if node.DashboardPort != nil {
		out = append(out, PortUse{Port: *node.DashboardPort, Kind: "dashboard"})
	}
	if node.KCPBindPort != nil {
		out = append(out, PortUse{Port: *node.KCPBindPort, Kind: "kcp"})
	}
	if node.QUICBindPort != nil {
		out = append(out, PortUse{Port: *node.QUICBindPort, Kind: "quic"})
	}

	var clients []model.FRPCClient
	if err := s.Store.DB.Where("frps_uuid = ?", frpsUUID).Find(&clients).Error; err != nil {
		return nil, err
	}
	for _, c := range clients {
		proxies, err := s.ListProxies(c.UUID)
		if err != nil {
			return nil, err
		}
		for _, p := range proxies {
			if p.RemotePort == nil {
				continue
			}
			out = append(out, PortUse{
				Port:      *p.RemotePort,
				Kind:      "proxy",
				FRPCUUID:  c.UUID,
				FRPCName:  c.Name,
				ProxyName: p.Name,
				ProxyType: p.ProxyType,
			})
		}
	}

	// External ports the agent reported as bound on the host (not assigned by us).
	hostOccupied, err := s.HostOccupiedPorts(frpsUUID)
	if err != nil {
		return nil, err
	}
	for _, p := range hostOccupied {
		out = append(out, PortUse{Port: p, Kind: "host"})
	}
	return out, nil
}
