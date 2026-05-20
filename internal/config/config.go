package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the control-plane server configuration loaded from YAML.
type Config struct {
	Server struct {
		Listen      string `yaml:"listen"`
		ExternalURL string `yaml:"external_url"`
	} `yaml:"server"`

	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`

	AgentAPIToken string `yaml:"agent_api_token"`
	JWTSecret     string `yaml:"jwt_secret"`

	PKI struct {
		CACert string `yaml:"ca_cert"`
		CAKey  string `yaml:"ca_key"`
	} `yaml:"pki"`

	FrpRelease struct {
		BaseURL        string `yaml:"base_url"`
		DefaultVersion string `yaml:"default_version"`
	} `yaml:"frp_release"`

	InstallScriptBase string `yaml:"install_script_base"`
	AgentDownloadBase string `yaml:"agent_download_base"`

	EnrollmentTokenTTL Duration `yaml:"enrollment_token_ttl"`

	CORS struct {
		AllowedOrigins []string `yaml:"allowed_origins"`
	} `yaml:"cors"`

	Logging struct {
		Level string `yaml:"level"`
		Path  string `yaml:"path"`
	} `yaml:"logging"`

	BootstrapAdmin struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"bootstrap_admin"`
}

// Duration wraps time.Duration so it can be parsed from YAML strings like "15m".
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) Std() time.Duration { return time.Duration(d) }

// Load reads and validates the configuration file.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Listen == "" {
		c.Server.Listen = "0.0.0.0:8443"
	}
	if c.FrpRelease.BaseURL == "" {
		c.FrpRelease.BaseURL = "https://github.com/fatedier/frp/releases/download"
	}
	if c.FrpRelease.DefaultVersion == "" {
		c.FrpRelease.DefaultVersion = "v0.61.1"
	}
	if c.EnrollmentTokenTTL == 0 {
		c.EnrollmentTokenTTL = Duration(15 * time.Minute)
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
}

func (c *Config) validate() error {
	if c.AgentAPIToken == "" || len(c.AgentAPIToken) < 16 {
		return fmt.Errorf("agent_api_token must be set (>=16 chars)")
	}
	if c.JWTSecret == "" || len(c.JWTSecret) < 16 {
		return fmt.Errorf("jwt_secret must be set (>=16 chars)")
	}
	if c.Database.Path == "" {
		return fmt.Errorf("database.path must be set")
	}
	if c.PKI.CACert == "" || c.PKI.CAKey == "" {
		return fmt.Errorf("pki.ca_cert and pki.ca_key must be set")
	}
	if c.Server.ExternalURL == "" {
		return fmt.Errorf("server.external_url must be set")
	}
	return nil
}
