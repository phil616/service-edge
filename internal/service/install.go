package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/dreamreflex/service-edge/internal/protocol"
	"github.com/dreamreflex/service-edge/scripts"
)

// installScriptData is the template context for install scripts.
type installScriptData struct {
	AgentType        string
	AgentConfig      string
	EnrollmentJSON   string
	UUID             string
	APIEndpoint      string
	APIToken         string
	EnrollmentToken  string
	AgentDownloadURL string
}

// RenderInstallScript renders the install script for the given enrollment token.
// The token is NOT consumed here (consumed at enroll time), only validated.
func (s *Service) RenderInstallScript(targetType, token string) (string, error) {
	tok, err := s.PeekEnrollment(token)
	if err != nil {
		return "", err
	}
	if tok.TargetType != targetType {
		return "", ErrEnrollmentInvalid
	}

	switch targetType {
	case "frps":
		_, err := s.GetFRPS(tok.TargetUUID)
		if err != nil {
			return "", err
		}
	case "frpc":
		_, err := s.GetFRPCHost(tok.TargetUUID)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unknown target type %q", targetType)
	}

	data := installScriptData{
		AgentType:        targetType,
		UUID:             tok.TargetUUID,
		APIEndpoint:      s.Cfg.Server.ExternalURL,
		APIToken:         protocol.AgentToken(s.Cfg.AgentAPIToken, targetType, tok.TargetUUID),
		EnrollmentToken:  token,
		AgentDownloadURL: s.AgentDownloadURL(targetType),
	}

	cfg, err := yaml.Marshal(map[string]any{
		"agent_type": targetType, "uuid": tok.TargetUUID, "api_endpoint": strings.TrimRight(s.Cfg.Server.ExternalURL, "/"), "api_token": protocol.AgentToken(s.Cfg.AgentAPIToken, targetType, tok.TargetUUID),
		"heartbeat_interval": "20s", "status_report_interval": "180s", "config_poll_timeout": "60s",
		"frp_binary_path": "/opt/service-edge/" + targetType + "-agent/bin/" + targetType,
	})
	if err != nil {
		return "", err
	}
	data.AgentConfig = string(cfg)
	enrollment, _ := json.Marshal(map[string]string{"uuid": tok.TargetUUID, "agent_type": targetType})
	data.EnrollmentJSON = string(enrollment)
	raw := scripts.FRPSInstall
	if targetType == "frpc" {
		raw = scripts.FRPCInstall
	}
	tmpl, err := template.New(targetType).Funcs(template.FuncMap{"shquote": func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }}).Parse(scripts.AgentInstall + raw)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// InstallCommand returns the one-liner users paste on the target host.
func (s *Service) InstallCommand(targetType, token string) string {
	base := strings.TrimRight(s.Cfg.InstallScriptBase, "/")
	return fmt.Sprintf("curl -fsSL %q | sudo bash", fmt.Sprintf("%s/%s.sh?token=%s", base, targetType, token))
}
