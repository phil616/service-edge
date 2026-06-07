package service

import (
	"path/filepath"
	"testing"

	"github.com/dreamreflex/service-edge/internal/model"
	"github.com/dreamreflex/service-edge/internal/store"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return &Service{Store: st, Notifier: NewNotifier()}
}

func seedInactiveProxy(t *testing.T, svc *Service, remotePort int) (connUUID string, proxyID uint) {
	t.Helper()
	if err := svc.Store.DB.Create(&model.FRPCHost{UUID: "host-1", Name: "h", FrpVersion: "v0.61.1"}).Error; err != nil {
		t.Fatalf("seed host: %v", err)
	}
	conn := &model.FRPCConnection{UUID: "conn-1", HostUUID: "host-1", FRPSUUID: "frps-1", Name: "c", AdminPort: 7400, ConfigVersion: 5}
	if err := svc.Store.DB.Create(conn).Error; err != nil {
		t.Fatalf("seed conn: %v", err)
	}
	rp := remotePort
	proxy := &model.ProxyMapping{
		FRPCUUID:       "conn-1",
		Name:           "ssh",
		ProxyType:      "tcp",
		LocalIP:        "127.0.0.1",
		LocalPort:      22,
		RemotePort:     &rp,
		Inactive:       true,
		InactiveReason: "occupied",
	}
	if err := svc.Store.DB.Create(proxy).Error; err != nil {
		t.Fatalf("seed proxy: %v", err)
	}
	return conn.UUID, proxy.ID
}

func connVersion(t *testing.T, svc *Service, uuid string) int {
	t.Helper()
	var c model.FRPCConnection
	if err := svc.Store.DB.Where("uuid = ?", uuid).First(&c).Error; err != nil {
		t.Fatalf("load conn: %v", err)
	}
	return c.ConfigVersion
}

func loadProxy(t *testing.T, svc *Service, id uint) model.ProxyMapping {
	t.Helper()
	var p model.ProxyMapping
	if err := svc.Store.DB.First(&p, id).Error; err != nil {
		t.Fatalf("load proxy: %v", err)
	}
	return p
}

// When the port is no longer bound on the host, a stuck-inactive mapping heals
// and its connection version is bumped so the change is delivered.
func TestReevaluateOccupancy_HealsWhenPortFreed(t *testing.T) {
	svc := newTestService(t)
	connUUID, pid := seedInactiveProxy(t, svc, 18000)

	svc.ReevaluateOccupancy("frps-1", []int{22000, 22001}) // 18000 absent => free

	p := loadProxy(t, svc, pid)
	if p.Inactive || p.InactiveReason != "" {
		t.Fatalf("expected proxy reactivated, got inactive=%v reason=%q", p.Inactive, p.InactiveReason)
	}
	if got := connVersion(t, svc, connUUID); got != 6 {
		t.Fatalf("expected connection version bumped 5->6, got %d", got)
	}
}

// When the port is still bound on the host, the mapping stays inactive and the
// connection version is left untouched (no spurious redeploys / no flapping).
func TestReevaluateOccupancy_KeepsInactiveWhilePortHeld(t *testing.T) {
	svc := newTestService(t)
	connUUID, pid := seedInactiveProxy(t, svc, 18000)

	svc.ReevaluateOccupancy("frps-1", []int{18000}) // still occupied

	p := loadProxy(t, svc, pid)
	if !p.Inactive {
		t.Fatal("expected proxy to stay inactive while port held")
	}
	if got := connVersion(t, svc, connUUID); got != 5 {
		t.Fatalf("expected connection version unchanged at 5, got %d", got)
	}
}
