package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/config"
)

func TestRenderProviderExplainText_ShowsChainAndProvenance(t *testing.T) {
	b := "builtin:codex"
	city := map[string]config.ProviderSpec{
		"codex-max": {
			Base:          &b,
			Command:       "aimux",
			Args:          []string{"run", "codex"},
			ReadyDelayMs:  5000,
			ResumeCommand: "aimux run codex -- resume {{.SessionKey}}",
			K8sCredentials: &config.ProviderK8sCredentials{
				Name:       "codex-max",
				SecretName: "codex-max-credentials",
				TargetDir:  ".codex-max",
			},
		},
	}
	resolved, err := config.ResolveProviderChain("codex-max", city["codex-max"], city)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	var out bytes.Buffer
	renderProviderExplainText(&out, resolved, "codex-max")
	got := out.String()

	if !strings.Contains(got, "Provider: codex-max") {
		t.Errorf("missing header: %s", got)
	}
	if !strings.Contains(got, "chain:") {
		t.Errorf("missing chain: %s", got)
	}
	if !strings.Contains(got, "builtin:codex") {
		t.Errorf("missing builtin:codex hop: %s", got)
	}
	if !strings.Contains(got, "# providers.codex-max") {
		t.Errorf("missing provenance annotation for leaf: %s", got)
	}
	if !strings.Contains(got, "k8s_credentials.secret_name") || !strings.Contains(got, "codex-max-credentials") {
		t.Errorf("missing k8s credential provenance: %s", got)
	}
}

func TestRenderProviderExplainJSON_PayloadShape(t *testing.T) {
	b := "builtin:codex"
	city := map[string]config.ProviderSpec{
		"codex-max": {
			Base:         &b,
			Command:      "aimux",
			ReadyDelayMs: 5000,
			OptionDefaults: map[string]string{
				"effort": "xhigh",
			},
			ResumeCommand: "aimux run codex -- resume {{.SessionKey}}",
			K8sCredentials: &config.ProviderK8sCredentials{
				Name:       "codex-max",
				SecretName: "codex-max-credentials",
				TargetDir:  ".codex-max",
				Env:        map[string]string{"CODEX_HOME": "{{.TargetDir}}"},
			},
		},
	}
	resolved, err := config.ResolveProviderChain("codex-max", city["codex-max"], city)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if rc := renderProviderExplainJSON(resolved, "codex-max", &stdout, &stderr); rc != 0 {
		t.Fatalf("rc = %d, stderr=%s", rc, stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json parse: %v — raw: %s", err, stdout.String())
	}

	if name, _ := payload["name"].(string); name != "codex-max" {
		t.Errorf("name = %v, want codex-max", payload["name"])
	}
	if payload["chain"] == nil {
		t.Errorf("chain missing: %v", payload)
	}
	prov, ok := payload["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("provenance not a map: %T", payload["provenance"])
	}
	fieldLayer, ok := prov["field_layer"].(map[string]any)
	if !ok {
		t.Fatalf("field_layer not a map: %T", prov["field_layer"])
	}
	if got := fieldLayer["command"]; got != "providers.codex-max" {
		t.Errorf("field_layer.command = %v, want providers.codex-max", got)
	}
	resolvedMap, ok := payload["resolved"].(map[string]any)
	if !ok {
		t.Fatalf("resolved not a map: %T", payload["resolved"])
	}
	k8sCreds, ok := resolvedMap["k8s_credentials"].(map[string]any)
	if !ok {
		t.Fatalf("k8s_credentials not a map: %T", resolvedMap["k8s_credentials"])
	}
	if got := k8sCreds["secret_name"]; got != "codex-max-credentials" {
		t.Errorf("k8s_credentials.secret_name = %v", got)
	}
	if got := fieldLayer["k8s_credentials"]; got != "providers.codex-max" {
		t.Errorf("field_layer.k8s_credentials = %v, want providers.codex-max", got)
	}
}
