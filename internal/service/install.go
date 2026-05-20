package service

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/dreamreflex/service-edge/scripts"
)

// installScriptData is the template context for install scripts.
type installScriptData struct {
	UUID             string
	APIEndpoint      string
	APIToken         string
	EnrollmentToken  string
	AgentDownloadURL string
	FrpVersion       string
	FrpBaseURL       string
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

	var frpVersion string
	switch targetType {
	case "frps":
		node, err := s.GetFRPS(tok.TargetUUID)
		if err != nil {
			return "", err
		}
		frpVersion = node.FrpVersion
	case "frpc":
		var c, err = s.GetFRPC(tok.TargetUUID)
		if err != nil {
			return "", err
		}
		frpVersion = c.FrpVersion
	default:
		return "", fmt.Errorf("unknown target type %q", targetType)
	}

	data := installScriptData{
		UUID:             tok.TargetUUID,
		APIEndpoint:      s.Cfg.Server.ExternalURL,
		APIToken:         s.Cfg.AgentAPIToken,
		EnrollmentToken:  token,
		AgentDownloadURL: s.Cfg.AgentDownloadBase,
		FrpVersion:       frpVersion,
		FrpBaseURL:       strings.TrimRight(s.Cfg.FrpRelease.BaseURL, "/"),
	}

	raw := scripts.FRPSInstall
	if targetType == "frpc" {
		raw = scripts.FRPCInstall
	}
	tmpl, err := template.New(targetType).Parse(raw)
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
