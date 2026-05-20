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

// Reload attempts a hot reload (SIGHUP) first, falling back to a full restart.
// Returns whether a restart was used and any error.
func (s Systemd) ReloadOrRestart(unit string) (restarted bool, err error) {
	if rerr := s.Reload(unit); rerr == nil {
		return false, nil
	}
	if rerr := s.Restart(unit); rerr != nil {
		return true, rerr
	}
	return true, nil
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
