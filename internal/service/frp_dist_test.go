package service

import (
	"testing"
	"time"

	"github.com/dreamreflex/service-edge/internal/model"
)

func TestCompareFrpVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int // sign only
	}{
		{"0.62.0", "0.61.1", 1},
		{"v0.61.1", "0.61.1", 0},
		{"0.61.1", "0.61.10", -1},
		{"0.9.0", "0.10.0", -1}, // numeric, not lexicographic
		{"1.0.0", "0.61.1", 1},
		{"0.61", "0.61.0", 0},
	}
	for _, c := range cases {
		got := compareFrpVersion(c.a, c.b)
		if (got > 0) != (c.want > 0) || (got < 0) != (c.want < 0) {
			t.Errorf("compareFrpVersion(%q,%q)=%d, want sign %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLatestFRPDistVersion(t *testing.T) {
	svc := newTestService(t)
	if got := svc.LatestFRPDistVersion(); got != "" {
		t.Fatalf("empty dist: want \"\", got %q", got)
	}

	for _, v := range []string{"0.61.1", "0.62.0", "0.9.0"} {
		row := model.FRPDistFile{Filename: "frp_" + v + "_linux_amd64.tar.gz", Version: v, OS: "linux", Arch: "amd64", CreatedAt: time.Now()}
		if err := svc.Store.DB.Create(&row).Error; err != nil {
			t.Fatalf("seed dist %s: %v", v, err)
		}
	}
	if got := svc.LatestFRPDistVersion(); got != "0.62.0" {
		t.Fatalf("want latest 0.62.0, got %q", got)
	}
}
