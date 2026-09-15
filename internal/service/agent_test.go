package service

import (
	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/protocol"
	"testing"
	"time"
)

func readConnection(t *testing.T, s *Service, u string) *model.FRPCConnection {
	t.Helper()
	c, e := s.GetConnection(u)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestConnectionLeaseDoesNotUseHostHeartbeatTTL(t *testing.T) {
	s := newTestService(t)
	u, _ := seedInactiveProxy(t, s, 18000)
	old := time.Now().Add(-80 * time.Second)
	if err := s.Store.DB.Model(&model.FRPCConnection{}).Where("uuid = ?", u).Updates(map[string]any{"status": "online", "last_heartbeat": old}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.RecordHeartbeat("frpc", "host-1", true); err != nil {
		t.Fatal(err)
	}
	s.ReapStaleAgents()
	if got := readConnection(t, s, u).Status; got != "online" {
		t.Fatalf("legacy connection expired before 180s report: %s", got)
	}
	expired := time.Now().Add(-time.Second)
	if err := s.Store.DB.Model(&model.FRPCConnection{}).Where("uuid = ?", u).Update("status_expires_at", expired).Error; err != nil {
		t.Fatal(err)
	}
	s.ReapStaleAgents()
	if got := readConnection(t, s, u).Status; got != "unknown" {
		t.Fatalf("stale observation must be unknown: %s", got)
	}
}
func TestProxyObservationDoesNotChangeDesiredConfiguration(t *testing.T) {
	s := newTestService(t)
	u, id := seedInactiveProxy(t, s, 18000)
	if err := s.Store.DB.Model(&model.ProxyMapping{}).Where("id = ?", id).Update("inactive", false).Error; err != nil {
		t.Fatal(err)
	}
	original := connVersion(t, s, u)
	for _, status := range []string{"start error", "check failed", "running"} {
		if err := s.applyProxyStatuses(u, []protocol.ProxyStatus{{Name: "ssh", Status: status, Err: "backend issue"}}); err != nil {
			t.Fatal(err)
		}
		p := loadProxy(t, s, id)
		if p.Inactive || p.ObservedStatus != status || connVersion(t, s, u) != original {
			t.Fatalf("observation mutated desired config: %+v", p)
		}
	}
}
func TestOldBinaryReportPreservesDesiredVersionAndAppliedRevision(t *testing.T) {
	s := newTestService(t)
	node := model.FRPSNode{UUID: "node", Name: "node", FrpVersion: "v0.68.0", ConfigVersion: 9}
	if err := s.Store.DB.Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	for _, revision := range []int{8, 7} {
		if err := s.RecordStatus("frps", "node", protocol.StatusRequest{ProcessAlive: true, ConfigVersion: revision, FrpVersion: "0.61.1"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetFRPS("node")
	if err != nil {
		t.Fatal(err)
	}
	if got.FrpVersion != "v0.68.0" || got.ConfigVersion != 9 || got.Runtime.BinaryVersion != "v0.61.1" || got.Runtime.AppliedConfigVersion != 8 {
		t.Fatalf("desired/observed corrupted: %+v", got)
	}
}
func TestConnectionReportsPreserveHostFactsAndRepresentFailures(t *testing.T) {
	s := newTestService(t)
	u, id := seedInactiveProxy(t, s, 18000)
	if err := s.Store.DB.Model(&model.ProxyMapping{}).Where("id = ?", id).Update("inactive", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.RecordStatus("frpc", "host-1", protocol.StatusRequest{ProcessAlive: true, SystemInfo: protocol.SystemInfo{OS: "linux", MemoryMB: 1024}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		cs   protocol.ConnectionStatus
		want string
	}{
		{"running", protocol.ConnectionStatus{ProcessAlive: true, ProcessStatusAvailable: true, ProxyStatusAvailable: true, ProxyStatuses: []protocol.ProxyStatus{{Name: "ssh", Status: "running"}}}, "online"},
		{"admin error", protocol.ConnectionStatus{ProcessAlive: true, ProcessStatusAvailable: true, StatusError: "admin: timeout"}, "unknown"},
		{"proxy failure", protocol.ConnectionStatus{ProcessAlive: true, ProcessStatusAvailable: true, ProxyStatusAvailable: true, ProxyStatuses: []protocol.ProxyStatus{{Name: "ssh", Status: "start error", Err: "port occupied"}}}, "degraded"},
		{"missing proxy", protocol.ConnectionStatus{ProcessAlive: true, ProcessStatusAvailable: true, ProxyStatusAvailable: true}, "unknown"},
		{"process stopped", protocol.ConnectionStatus{ProcessStatusAvailable: true}, "offline"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.cs.UUID = u
			tc.cs.ConfigVersion = 5
			if err := s.RecordStatus("frpc", "host-1", protocol.StatusRequest{ProcessAlive: true, ConnectionsOnly: true, ConnectionReportIntervalSeconds: 10, Connections: []protocol.ConnectionStatus{tc.cs}}); err != nil {
				t.Fatal(err)
			}
			c := readConnection(t, s, u)
			if c.Status != tc.want || c.AppliedConfigVersion != 5 {
				t.Fatalf("%+v", c)
			}
			if c.StatusExpiresAt == nil || time.Until(*c.StatusExpiresAt) < 55*time.Second {
				t.Fatal("missing modern lease")
			}
			p := loadProxy(t, s, id)
			if p.Inactive {
				t.Fatal("observation disabled proxy")
			}
		})
	}
	h, err := s.GetFRPCHost("host-1")
	if err != nil {
		t.Fatal(err)
	}
	if h.Runtime.OS != "linux" || h.Runtime.MemoryMB != 1024 {
		t.Fatal("lightweight report erased host facts")
	}
}
func TestConnectionReportCannotModifyAnotherHost(t *testing.T) {
	s := newTestService(t)
	u, id := seedInactiveProxy(t, s, 18000)
	if err := s.Store.DB.Create(&model.FRPCHost{UUID: "other", Name: "other"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.RecordStatus("frpc", "other", protocol.StatusRequest{ProcessAlive: true, Connections: []protocol.ConnectionStatus{{UUID: u, ProcessAlive: true, ProxyStatusAvailable: true, ProxyStatuses: []protocol.ProxyStatus{{Name: "ssh", Status: "running"}}}}}); err != nil {
		t.Fatal(err)
	}
	if readConnection(t, s, u).Status != "pending" || loadProxy(t, s, id).ObservedStatus != "" {
		t.Fatal("report crossed host boundary")
	}
}
func TestAppliedVersionAndAckAreDurableAndMonotonic(t *testing.T) {
	s := newTestService(t)
	if err := s.Store.DB.Create(&model.FRPCHost{UUID: "h", Name: "h", ConfigVersion: 5}).Error; err != nil {
		t.Fatal(err)
	}
	for _, ack := range []protocol.AckRequest{{ConfigVersion: 5, Success: true}, {ConfigVersion: 4, Success: false, Error: "late failure"}, {ConfigVersion: 5, Success: false, Error: "late failure at same revision"}} {
		if err := s.RecordConfigAck("frpc", "h", ack); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordAppliedVersion("frpc", "h", 3); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAppliedVersion("frpc", "h", 99); err != nil {
		t.Fatal(err)
	}
	h, e := s.GetFRPCHost("h")
	if e != nil {
		t.Fatal(e)
	}
	if h.Runtime.AppliedConfigVersion != 5 || h.Runtime.LastApplyError != "" || h.Runtime.LastApplyVersion != 5 {
		t.Fatal(h.Runtime)
	}
}
func TestStatusDatabaseFailureIsReturned(t *testing.T) {
	s := newTestService(t)
	u, _ := seedInactiveProxy(t, s, 18000)
	if err := s.Store.DB.Exec("DROP TABLE proxy_mappings").Error; err != nil {
		t.Fatal(err)
	}
	err := s.RecordStatus("frpc", "host-1", protocol.StatusRequest{ProcessAlive: true, Connections: []protocol.ConnectionStatus{{UUID: u}}})
	if err == nil {
		t.Fatal("database failure acknowledged")
	}
	var h model.FRPCHost
	if err := s.Store.DB.Where("uuid = ?", "host-1").First(&h).Error; err != nil {
		t.Fatal(err)
	}
	if h.LastHeartbeat != nil {
		t.Fatal("failed report partially committed")
	}
}

func TestHeartbeatCompensatesForLostSuccessAck(t *testing.T) {
	s := newTestService(t)
	if err := s.Store.DB.Create(&model.FRPCHost{UUID: "h", Name: "h", ConfigVersion: 5}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.RecordConfigAck("frpc", "h", protocol.AckRequest{ConfigVersion: 5, Error: "previous attempt failed"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAppliedVersion("frpc", "h", 5); err != nil {
		t.Fatal(err)
	}
	h, err := s.GetFRPCHost("h")
	if err != nil {
		t.Fatal(err)
	}
	if h.Runtime.AppliedConfigVersion != 5 || h.Runtime.LastApplyError != "" {
		t.Fatal(h.Runtime)
	}
}

func TestLegacyProxySnapshotRemainsReadable(t *testing.T) {
	s := newTestService(t)
	u, id := seedInactiveProxy(t, s, 18000)
	if err := s.Store.DB.Model(&model.ProxyMapping{}).Where("id = ?", id).Update("inactive", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.RecordStatus("frpc", "host-1", protocol.StatusRequest{ProcessAlive: true, Connections: []protocol.ConnectionStatus{{UUID: u, ProcessAlive: true, ProxyStatuses: []protocol.ProxyStatus{{Name: "ssh", Status: "running"}}}}}); err != nil {
		t.Fatal(err)
	}
	conn := readConnection(t, s, u)
	if conn.Status != "online" || conn.StatusExpiresAt == nil || time.Until(*conn.StatusExpiresAt) < 230*time.Second {
		t.Fatal(conn)
	}
}
