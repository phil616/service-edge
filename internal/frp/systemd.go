package frp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func (s Systemd) DaemonReload() error      { _, err := s.run("daemon-reload"); return err }
func (s Systemd) Enable(unit string) error { _, err := s.run("enable", unit); return err }
func (s Systemd) Disable(unit string) error {
	_, err := s.run("disable", unit)
	if err != nil && s.unitMissing(unit) {
		return nil
	}
	return err
}
func (s Systemd) Start(unit string) error { _, err := s.run("start", unit); return err }
func (s Systemd) Stop(unit string) error {
	_, err := s.run("stop", unit)
	if err != nil && s.unitMissing(unit) {
		return nil
	}
	return err
}
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

// Configure owns each FRP unit. ExecStart follows the atomically switched
// generation, binding its executable and certificates to its configuration.
func (s Systemd) Configure(unit, binary, config string) error {
	if strings.ContainsAny(unit, "/\\\n\r") || strings.ContainsAny(binary+config, "\n\r") {
		return fmt.Errorf("invalid managed unit path")
	}
	name := unit
	if !strings.HasSuffix(name, ".service") {
		name += ".service"
	}
	path := filepath.Join("/etc/systemd/system", name)
	content := fmt.Sprintf("[Unit]\nDescription=Service Edge managed FRP\nWants=network-online.target\nAfter=network-online.target\nStartLimitIntervalSec=0\n\n[Service]\nType=simple\nExecStart=%s -c %s\nRestart=on-failure\nRestartSec=5s\n\n[Install]\nWantedBy=multi-user.target\n", strconv.Quote(binary), strconv.Quote(config))
	if old, err := os.ReadFile(path); err == nil && string(old) == content {
		return s.DaemonReload()
	}
	if err := os.WriteFile(path+".new", []byte(content), 0644); err != nil {
		return err
	}
	if err := os.Rename(path+".new", path); err != nil {
		return err
	}
	return s.DaemonReload()
}

// A first apply can fail before creating a unit. Deleting that connection must
// converge too, while real systemd/permission failures remain retryable errors.
func (s Systemd) unitMissing(unit string) bool {
	out, err := s.run("show", "--property=LoadState", "--value", unit)
	return err == nil && strings.TrimSpace(out) == "not-found"
}
