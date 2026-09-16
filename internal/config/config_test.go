package config

import "testing"

func TestPublicURLsDeriveFromControlPlane(t *testing.T) {
	c := Config{}
	c.Server.ExternalURL = " https://edge.example.com/ "
	c.applyDefaults()
	if c.Server.ExternalURL != "https://edge.example.com" || c.InstallScriptBase != "https://edge.example.com/install" || c.AgentDownloadBase != "https://edge.example.com/download/agent" {
		t.Fatalf("%+v", c)
	}
	c.AgentDownloadBase = "https://cdn.example.com/agent"
	c.applyDefaults()
	if c.AgentDownloadBase != "https://cdn.example.com/agent" {
		t.Fatal("explicit download override lost")
	}
}
