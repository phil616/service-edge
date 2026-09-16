package service

import (
	"github.com/dreamreflex/service-edge/internal/model"
	"testing"
)

func TestConfigurationMutationRollsBackWhenHostRevisionFails(t *testing.T) {
	for _, operation := range []string{"add", "update", "delete", "reactivate", "connection", "node"} {
		t.Run(operation, func(t *testing.T) {
			svc := newTestService(t)
			conn, id := seedInactiveProxy(t, svc, 18000)
			node := model.FRPSNode{UUID: "frps-1", Name: "original", BindPort: 7000, ConfigVersion: 3}
			if err := svc.Store.DB.Create(&node).Error; err != nil {
				t.Fatal(err)
			}
			if err := svc.Store.DB.Exec(`CREATE TRIGGER fail_revision BEFORE UPDATE OF config_version ON f_rpc_hosts BEGIN SELECT RAISE(FAIL, 'injected revision failure'); END`).Error; err != nil {
				t.Fatal(err)
			}
			port := 19000
			input := ProxyMappingInput{Name: "new", ProxyType: "tcp", LocalPort: 22, RemotePort: &port}
			var err error
			switch operation {
			case "add":
				_, err = svc.AddProxy(conn, input)
			case "update":
				_, err = svc.UpdateProxy(id, input)
			case "delete":
				err = svc.DeleteProxy(id)
			case "reactivate":
				svc.ReevaluateOccupancy(node.UUID, nil)
			case "connection":
				err = svc.DeleteConnection(conn)
			case "node":
				name := "changed"
				_, err = svc.UpdateFRPS(node.UUID, UpdateFRPSInput{Name: &name})
			}
			if operation != "reactivate" && err == nil {
				t.Fatal("injected failure was ignored")
			}
			if got := connVersion(t, svc, conn); got != 5 {
				t.Fatalf("revision partially committed: %d", got)
			}
			p := loadProxy(t, svc, id)
			if p.Name != "ssh" || !p.Inactive || *p.RemotePort != 18000 {
				t.Fatalf("proxy partially committed: %+v", p)
			}
			proxies, err := svc.ListProxies(conn)
			if err != nil || len(proxies) != 1 {
				t.Fatalf("unexpected proxies: %v %v", proxies, err)
			}
			got, err := svc.GetFRPS(node.UUID)
			if err != nil || got.Name != "original" || got.ConfigVersion != 3 {
				t.Fatalf("node partially committed: %+v %v", got, err)
			}
		})
	}
}

func TestProxyNameAndPortValidation(t *testing.T) {
	svc := newTestService(t)
	conn, _ := seedInactiveProxy(t, svc, 18000)
	if err := svc.Store.DB.Create(&model.FRPSNode{UUID: "frps-1", BindPort: 7000}).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name          string
		local, remote int
	}{{"ssh", 22, 19000}, {"different", 65536, 19000}, {"different", 22, 65536}} {
		_, err := svc.AddProxy(conn, ProxyMappingInput{Name: tc.name, ProxyType: "tcp", LocalPort: tc.local, RemotePort: &tc.remote})
		if err == nil {
			t.Fatalf("invalid proxy accepted: %+v", tc)
		}
	}
}

func TestDirectNodeRejectsWSSAndDisabledTransport(t *testing.T) {
	node := model.FRPSNode{BindPort: 7000}
	for _, transport := range []string{ProtoWSS, ProtoKCP, ProtoQUIC} {
		if _, err := validateClientProtocol(node, transport); err == nil {
			t.Fatalf("unsupported direct transport accepted: %s", transport)
		}
	}
	for _, transport := range []string{ProtoTCP, ProtoWebsocket} {
		if _, err := validateClientProtocol(node, transport); err != nil {
			t.Fatal(err)
		}
	}
}
