package frp

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Systemd controls frp via systemctl. Methods are thin wrappers that surface
// the command output on failure for logging.
type Systemd struct{}

func (Systemd) run(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (s Systemd) DaemonReload() error       { _, err := s.run("daemon-reload"); return err }
func (s Systemd) Enable(unit string) error  { _, err := s.run("enable", unit); return err }
func (s Systemd) Disable(unit string) error { _, err := s.run("disable", unit); return err }
func (s Systemd) Start(unit string) error   { _, err := s.run("start", unit); return err }
func (s Systemd) Stop(unit string) error    { _, err := s.run("stop", unit); return err }
func (s Systemd) Restart(unit string) error { _, err := s.run("restart", unit); return err }

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

// ProcessStatus reads both fields in one bounded systemctl call and preserves
// observation errors instead of reporting a stopped process on command failure.
func (s Systemd) ProcessStatus(ctx context.Context, unit string) (bool, int, error) {
	out, err := exec.CommandContext(ctx, "systemctl", "show", "--property=ActiveState,MainPID", unit).CombinedOutput()
	if err != nil {
		return false, 0, fmt.Errorf("systemctl show: %w", err)
	}
	var state string
	var pid int
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "ActiveState":
			state = value
		case "MainPID":
			pid, err = strconv.Atoi(value)
			if err != nil {
				return false, 0, err
			}
		}
	}
	if state == "" {
		return false, 0, fmt.Errorf("missing ActiveState")
	}
	return state == "active", pid, nil
}
