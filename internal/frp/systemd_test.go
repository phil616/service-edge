package frp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemovingNeverCreatedUnitIsIdempotent(t *testing.T) {
	for _, state := range []string{"not-found", "loaded", "unavailable"} {
		t.Run(state, func(t *testing.T) {
			dir := t.TempDir()
			script := "#!/bin/sh\nif [ \"$1\" = show ]; then\n"
			if state == "unavailable" {
				script += "exit 1\n"
			} else {
				script += "echo " + state + "\nexit 0\n"
			}
			script += "fi\necho 'operation failed' >&2\nexit 1\n"
			if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			for _, operation := range []func(string) error{(Systemd{}).Stop, (Systemd{}).Disable} {
				err := operation("service-edge-frpc@test")
				if (err == nil) != (state == "not-found") {
					t.Fatal(state, err)
				}
			}
		})
	}
}
