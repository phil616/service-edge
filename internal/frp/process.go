package frp

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// VerifyConfig runs `<binary> verify -c <configPath>`. frp (v0.46+) provides a
// `verify` subcommand for both frps and frpc that validates config syntax
// without starting the service.
func VerifyConfig(binaryPath, configPath string) error {
	cmd := exec.Command(binaryPath, "verify", "-c", configPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("config verify failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// FrpVersion runs `<binary> --version` and returns the trimmed output.
func FrpVersion(binaryPath string) string {
	cmd := exec.Command(binaryPath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// ReloadOrRestart applies a staged config change to the running frp process by
// fully restarting it.
//
// We deliberately do NOT use SIGHUP "hot reload": frp does not hot-reload proxies
// on SIGHUP (its only hot-reload path is the admin API, and even that skips
// transport/TLS changes), while `systemctl reload` -> `kill -HUP` always reports
// success regardless of whether frp did anything. That combination silently
// accepted every config change at the control plane while the data plane kept the
// old proxy set — e.g. a deleted remote_port stayed bound. A restart is cheap
// (~1-2s) and is the only reliable way to make frp adopt added/removed proxies.
//
// Returns restarted=true (kept for caller logging).
func (s Systemd) ReloadOrRestart(unit string) (restarted bool, err error) {
	return true, s.Restart(unit)
}

// WaitActive polls is-active for up to timeout, returning true once active.
func (s Systemd) WaitActive(unit string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.IsActive(unit) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return s.IsActive(unit)
}
