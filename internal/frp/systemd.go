package frp

import (
	"fmt"
	"os/exec"
	"strings"
)

// Systemd controls frp via systemctl. Methods are thin wrappers that surface
// the command output on failure for logging.
type Systemd struct{}

func (Systemd) run(args ...string) (string, error) {
	cmd := exec.Command("systemctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (s Systemd) DaemonReload() error       { _, err := s.run("daemon-reload"); return err }
func (s Systemd) Enable(unit string) error  { _, err := s.run("enable", unit); return err }
func (s Systemd) Disable(unit string) error { _, err := s.run("disable", unit); return err }
func (s Systemd) Start(unit string) error  { _, err := s.run("start", unit); return err }
func (s Systemd) Stop(unit string) error   { _, err := s.run("stop", unit); return err }
func (s Systemd) Restart(unit string) error { _, err := s.run("restart", unit); return err }

// Reload sends SIGHUP via systemd (ExecReload). frp v0.50+ honors SIGHUP for
// hot reload of proxies; transport/TLS changes still need a restart.
func (s Systemd) Reload(unit string) error { _, err := s.run("reload", unit); return err }

// IsActive reports whether the unit is currently active (running).
func (s Systemd) IsActive(unit string) bool {
	out, _ := s.run("is-active", unit)
	return strings.TrimSpace(out) == "active"
}

// MainPID returns the systemd-reported main PID of the unit (0 if unknown).
func (s Systemd) MainPID(unit string) int {
	out, err := s.run("show", "-p", "MainPID", "--value", unit)
	if err != nil {
		return 0
	}
	var pid int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &pid)
	return pid
}
