package k8s

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/gastownhall/gascity/internal/runtime"
)

func TestBuildPod_NodeSelector(t *testing.T) {
	p := newProviderWithOps(newFakeK8sOps())
	p.nodeSelector = map[string]string{"workload": "gc-agents"}
	pod, err := buildPod("test-session", runtime.Config{Command: "/bin/bash"}, p)
	if err != nil {
		t.Fatalf("buildPod: %v", err)
	}
	if pod.Spec.NodeSelector["workload"] != "gc-agents" {
		t.Errorf("NodeSelector[workload] = %q, want \"gc-agents\"", pod.Spec.NodeSelector["workload"])
	}
}

func TestBuildPod_Tolerations(t *testing.T) {
	p := newProviderWithOps(newFakeK8sOps())
	p.tolerations = []corev1.Toleration{{
		Key: "gc-agents", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule,
	}}
	pod, err := buildPod("test-session", runtime.Config{Command: "/bin/bash"}, p)
	if err != nil {
		t.Fatalf("buildPod: %v", err)
	}
	if len(pod.Spec.Tolerations) != 1 {
		t.Fatalf("len(Tolerations) = %d, want 1", len(pod.Spec.Tolerations))
	}
	if pod.Spec.Tolerations[0].Key != "gc-agents" {
		t.Errorf("Toleration.Key = %q, want \"gc-agents\"", pod.Spec.Tolerations[0].Key)
	}
}

func TestBuildPod_Affinity(t *testing.T) {
	p := newProviderWithOps(newFakeK8sOps())
	p.affinity = &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key: "node-type", Operator: corev1.NodeSelectorOpIn, Values: []string{"gpu"},
					}},
				}},
			},
		},
	}
	pod, err := buildPod("test-session", runtime.Config{Command: "/bin/bash"}, p)
	if err != nil {
		t.Fatalf("buildPod: %v", err)
	}
	if pod.Spec.Affinity == nil {
		t.Fatal("Affinity is nil")
	}
	if pod.Spec.Affinity.NodeAffinity == nil {
		t.Fatal("NodeAffinity is nil")
	}
	expressions := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions
	if expressions[0].Values[0] != "gpu" {
		t.Fatalf("affinity value = %q, want gpu", expressions[0].Values[0])
	}
}

func TestBuildPod_PriorityClassName(t *testing.T) {
	p := newProviderWithOps(newFakeK8sOps())
	p.priorityClassName = "gc-agent-high"
	pod, err := buildPod("test-session", runtime.Config{Command: "/bin/bash"}, p)
	if err != nil {
		t.Fatalf("buildPod: %v", err)
	}
	if pod.Spec.PriorityClassName != "gc-agent-high" {
		t.Errorf("PriorityClassName = %q, want \"gc-agent-high\"", pod.Spec.PriorityClassName)
	}
}

func TestBuildPod_NoSchedulingFields_NoBehaviorChange(t *testing.T) {
	// Zero-value scheduling fields must not alter default pod behavior.
	p := newProviderWithOps(newFakeK8sOps())
	pod, err := buildPod("test-session", runtime.Config{Command: "/bin/bash"}, p)
	if err != nil {
		t.Fatalf("buildPod: %v", err)
	}
	if pod.Spec.NodeSelector != nil {
		t.Errorf("NodeSelector should be nil when not set")
	}
	if len(pod.Spec.Tolerations) != 0 {
		t.Errorf("Tolerations should be empty when not set")
	}
	if pod.Spec.Affinity != nil {
		t.Errorf("Affinity should be nil when not set")
	}
	if pod.Spec.PriorityClassName != "" {
		t.Errorf("PriorityClassName should be empty when not set")
	}
}

func TestBuildPod_ClonesSchedulingFields(t *testing.T) {
	seconds := int64(30)
	p := newProviderWithOps(newFakeK8sOps())
	p.nodeSelector = map[string]string{"workload": "gc-agents"}
	p.tolerations = []corev1.Toleration{{
		Key:               "gc-agents",
		Operator:          corev1.TolerationOpExists,
		Effect:            corev1.TaintEffectNoSchedule,
		TolerationSeconds: &seconds,
	}}
	p.affinity = &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key: "node-type", Operator: corev1.NodeSelectorOpIn, Values: []string{"gpu"},
					}},
				}},
			},
		},
	}

	pod, err := buildPod("test-session", runtime.Config{Command: "/bin/bash"}, p)
	if err != nil {
		t.Fatalf("buildPod: %v", err)
	}

	pod.Spec.NodeSelector["workload"] = "changed"
	pod.Spec.Tolerations[0].Key = "changed"
	pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0] = "changed"

	if p.nodeSelector["workload"] != "gc-agents" {
		t.Fatalf("provider nodeSelector mutated to %q", p.nodeSelector["workload"])
	}
	if p.tolerations[0].Key != "gc-agents" {
		t.Fatalf("provider toleration key mutated to %q", p.tolerations[0].Key)
	}
	values := p.affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values
	if values[0] != "gpu" {
		t.Fatalf("provider affinity value mutated to %q", values[0])
	}
}

func TestBuildPod_WaitsForEitherWorkspaceReadyMarker(t *testing.T) {
	p := newProviderWithOps(newFakeK8sOps())
	pod, err := buildPod("test-session", runtime.Config{Command: "/bin/bash"}, p)
	if err != nil {
		t.Fatalf("buildPod: %v", err)
	}

	args := pod.Spec.Containers[0].Args
	if len(args) != 1 {
		t.Fatalf("container args = %v, want one shell command", args)
	}
	if !strings.Contains(args[0], `/workspace/.gc-workspace-ready ] && [ ! -f /workspace/.gc-ready`) {
		t.Fatalf("entrypoint does not accept both workspace ready markers: %s", args[0])
	}
}

func TestBuildPod_MountsProviderCredentialSecrets(t *testing.T) {
	p := newProviderWithOps(newFakeK8sOps())
	pod, err := buildPod("test-session", runtime.Config{Command: "/bin/bash"}, p)
	if err != nil {
		t.Fatalf("buildPod: %v", err)
	}

	claudeMount, ok := volumeMountByName(pod.Spec.Containers[0].VolumeMounts, "claude-config")
	if !ok {
		t.Fatal("missing claude-config volume mount")
	}
	if claudeMount.MountPath != "/tmp/claude-secret" || !claudeMount.ReadOnly {
		t.Fatalf("claude-config mount = %#v, want readonly /tmp/claude-secret", claudeMount)
	}
	claudeVolume, ok := volumeByName(pod.Spec.Volumes, "claude-config")
	if !ok || claudeVolume.Secret == nil {
		t.Fatalf("missing claude-config secret volume: %#v", claudeVolume)
	}
	if claudeVolume.Secret.SecretName != "claude-credentials" {
		t.Fatalf("claude secret name = %q, want claude-credentials", claudeVolume.Secret.SecretName)
	}
	if claudeVolume.Secret.Optional == nil || !*claudeVolume.Secret.Optional {
		t.Fatal("claude secret should be optional")
	}

	codexMount, ok := volumeMountByName(pod.Spec.Containers[0].VolumeMounts, "codex-config")
	if !ok {
		t.Fatal("missing codex-config volume mount")
	}
	if codexMount.MountPath != "/tmp/codex-secret" || !codexMount.ReadOnly {
		t.Fatalf("codex-config mount = %#v, want readonly /tmp/codex-secret", codexMount)
	}
	codexVolume, ok := volumeByName(pod.Spec.Volumes, "codex-config")
	if !ok || codexVolume.Secret == nil {
		t.Fatalf("missing codex-config secret volume: %#v", codexVolume)
	}
	if codexVolume.Secret.SecretName != "codex-credentials" {
		t.Fatalf("codex secret name = %q, want codex-credentials", codexVolume.Secret.SecretName)
	}
	if codexVolume.Secret.Optional == nil || !*codexVolume.Secret.Optional {
		t.Fatal("codex secret should be optional")
	}
}

func TestBuildPodEnv_ProviderCredentialEnvUsesConfiguredContainerHome(t *testing.T) {
	env, err := buildPodEnv(
		map[string]string{"GC_K8S_CONTAINER_HOME": "/home/gascity"},
		"/workspace",
		podManagedDoltHost,
		podManagedDoltPort,
	)
	if err != nil {
		t.Fatalf("buildPodEnv: %v", err)
	}

	if got := envValue(env, "CLAUDE_CONFIG_DIR"); got != "/home/gascity/.claude" {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want /home/gascity/.claude", got)
	}
	if got := envValue(env, "CODEX_HOME"); got != "/home/gascity/.codex" {
		t.Fatalf("CODEX_HOME = %q, want /home/gascity/.codex", got)
	}
}

func TestBuildPodEnv_ProviderCredentialEnvUsesDynamicLinuxUser(t *testing.T) {
	env, err := buildPodEnv(
		map[string]string{"LINUX_USERNAME": "alice", "GC_K8S_CONTAINER_HOME": "/home/gascity"},
		"/workspace",
		podManagedDoltHost,
		podManagedDoltPort,
	)
	if err != nil {
		t.Fatalf("buildPodEnv: %v", err)
	}

	if got := envValue(env, "CLAUDE_CONFIG_DIR"); got != "/home/alice/.claude" {
		t.Fatalf("CLAUDE_CONFIG_DIR = %q, want /home/alice/.claude", got)
	}
	if got := envValue(env, "CODEX_HOME"); got != "/home/alice/.codex" {
		t.Fatalf("CODEX_HOME = %q, want /home/alice/.codex", got)
	}
}

func TestBuildPodEnv_GitHubTokenEnvSupportsGitAndGHCLI(t *testing.T) {
	env, err := buildPodEnv(map[string]string{}, "/workspace", podManagedDoltHost, podManagedDoltPort)
	if err != nil {
		t.Fatalf("buildPodEnv: %v", err)
	}

	for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		v, ok := envByName(env, name)
		if !ok {
			t.Fatalf("missing %s env", name)
		}
		if v.ValueFrom == nil || v.ValueFrom.SecretKeyRef == nil {
			t.Fatalf("%s does not come from a secret: %#v", name, v)
		}
		ref := v.ValueFrom.SecretKeyRef
		if ref.Name != "git-credentials" || ref.Key != "token" {
			t.Fatalf("%s secret ref = %s/%s, want git-credentials/token", name, ref.Name, ref.Key)
		}
		if ref.Optional == nil || !*ref.Optional {
			t.Fatalf("%s secret ref should be optional", name)
		}
	}
}

func TestBuildPod_CredentialBootstrapCopiesClaudeRootConfig(t *testing.T) {
	p := newProviderWithOps(newFakeK8sOps())
	pod, err := buildPod("test-session", runtime.Config{Command: "/bin/bash"}, p)
	if err != nil {
		t.Fatalf("buildPod: %v", err)
	}
	args := pod.Spec.Containers[0].Args
	if len(args) != 1 {
		t.Fatalf("container args = %v, want one shell command", args)
	}
	if !strings.Contains(args[0], `cp -f /tmp/claude-secret/.claude.json "$HOME/.claude.json"`) {
		t.Fatalf("credential bootstrap does not copy Claude root config: %s", args[0])
	}
}

func volumeMountByName(mounts []corev1.VolumeMount, name string) (corev1.VolumeMount, bool) {
	for _, mount := range mounts {
		if mount.Name == name {
			return mount, true
		}
	}
	return corev1.VolumeMount{}, false
}

func volumeByName(volumes []corev1.Volume, name string) (corev1.Volume, bool) {
	for _, volume := range volumes {
		if volume.Name == name {
			return volume, true
		}
	}
	return corev1.Volume{}, false
}

func envByName(env []corev1.EnvVar, name string) (corev1.EnvVar, bool) {
	for _, item := range env {
		if item.Name == name {
			return item, true
		}
	}
	return corev1.EnvVar{}, false
}

func envValue(env []corev1.EnvVar, name string) string {
	item, ok := envByName(env, name)
	if !ok {
		return ""
	}
	return item.Value
}
