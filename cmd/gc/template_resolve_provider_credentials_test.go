package main

import (
	"testing"

	"github.com/gastownhall/gascity/internal/config"
)

func TestTemplateParamsToConfigCarriesProviderK8sCredentials(t *testing.T) {
	required := false
	cfg := templateParamsToConfig(TemplateParams{
		Command: "codex",
		ResolvedProvider: &config.ResolvedProvider{
			Name: "codex-polecat",
			K8sCredentials: &config.ProviderK8sCredentials{
				Name:       "codex-polecat",
				SecretName: "codex-polecat-credentials",
				TargetDir:  ".codex-polecat",
				Optional:   &required,
				Env:        map[string]string{"CODEX_HOME": "{{.TargetDir}}"},
				EnvFromSecret: []config.ProviderK8sSecretEnv{{
					Name: "CODEX_SESSION_TOKEN",
					Key:  "session-token",
				}},
			},
		},
	})

	if len(cfg.ProviderCredentials) != 1 {
		t.Fatalf("ProviderCredentials len = %d, want 1", len(cfg.ProviderCredentials))
	}
	profile := cfg.ProviderCredentials[0]
	if profile.Name != "codex-polecat" || profile.SecretName != "codex-polecat-credentials" || profile.Optional {
		t.Fatalf("ProviderCredentials[0] = %#v", profile)
	}
	if got := profile.Env["CODEX_HOME"]; got != "{{.TargetDir}}" {
		t.Fatalf("CODEX_HOME env = %q", got)
	}
	if len(profile.EnvFromSecret) != 1 || profile.EnvFromSecret[0].Key != "session-token" {
		t.Fatalf("EnvFromSecret = %#v", profile.EnvFromSecret)
	}
}
