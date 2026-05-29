package k8s

import (
	"encoding/base64"
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

func TestBuildPodEntrypointLaunchesTmuxFromWorkDirAfterPreStart(t *testing.T) {
	p := newProviderWithOps(newFakeK8sOps())
	pod, err := buildPod("test-session", runtime.Config{
		Command:  "codex",
		WorkDir:  "/city/rigs/frontend",
		PreStart: []string{"rm -rf /workspace/rigs/frontend && mkdir -p /workspace/rigs/frontend"},
		Env: map[string]string{
			"GC_CITY": "/city",
		},
	}, p)
	if err != nil {
		t.Fatalf("buildPod: %v", err)
	}

	args := pod.Spec.Containers[0].Args
	if len(args) != 1 {
		t.Fatalf("container args = %v, want one shell command", args)
	}
	if got := pod.Spec.Containers[0].WorkingDir; got != podEntrypointWorkDir {
		t.Fatalf("container workingDir = %q, want stable entrypoint dir %q", got, podEntrypointWorkDir)
	}
	entrypoint := args[0]
	preStartIdx := strings.Index(entrypoint, "base64 -d | sh")
	launchIdx := strings.Index(entrypoint, "cd '/workspace/rigs/frontend' && tmux new-session")
	if preStartIdx == -1 {
		t.Fatalf("entrypoint does not run pre_start via base64 shell: %s", entrypoint)
	}
	if launchIdx == -1 {
		t.Fatalf("entrypoint does not cd to projected workdir before tmux: %s", entrypoint)
	}
	if preStartIdx > launchIdx {
		t.Fatalf("entrypoint cd happens before pre_start; want pre_start then final cd: %s", entrypoint)
	}
}

func TestBuildPodPersistentWorkspacePVCUsesMountedCityRoot(t *testing.T) {
	p := newProviderWithOps(newFakeK8sOps())
	p.workspacePVC = "demo-city-workspace"
	p.workspaceRoot = "/workspace/cities/demo-city"

	cfg := runtime.Config{
		Command:  "gc agent-script --script /host/city/packs/demo/agent.yaml",
		WorkDir:  "/host/city/.gc/worktrees/demo/cartographer",
		PreStart: []string{"test -d /host/city/rigs/demo"},
		Env: map[string]string{
			"GC_AGENT":             "demo/cartographer",
			"GC_CITY":              "/host/city",
			"GC_CITY_PATH":         "/host/city",
			"GC_DIR":               "/host/city/.gc/worktrees/demo/cartographer",
			"GC_RIG_ROOT":          "/host/city/rigs/demo",
			"GC_STORE_ROOT":        "/host/city/rigs/demo",
			"BEADS_DIR":            "/host/city/rigs/demo/.beads",
			"GC_CITY_RUNTIME_DIR":  "/host/city/.gc/runtime",
			"GC_K8S_WORKSPACE_PVC": "demo-city-workspace",
		},
	}

	pod, err := buildPod("test-session", cfg, p)
	if err != nil {
		t.Fatalf("buildPod: %v", err)
	}
	if len(pod.Spec.InitContainers) != 0 {
		t.Fatalf("persistent workspace pod should not use init staging containers: %#v", pod.Spec.InitContainers)
	}

	workspaceMount, ok := volumeMountByName(pod.Spec.Containers[0].VolumeMounts, "workspace")
	if !ok {
		t.Fatal("missing persistent workspace volume mount")
	}
	if workspaceMount.MountPath != "/workspace/cities/demo-city" {
		t.Fatalf("workspace mount path = %q, want /workspace/cities/demo-city", workspaceMount.MountPath)
	}
	workspaceVolume, ok := volumeByName(pod.Spec.Volumes, "workspace")
	if !ok || workspaceVolume.PersistentVolumeClaim == nil {
		t.Fatalf("missing persistent workspace PVC volume: %#v", workspaceVolume)
	}
	if workspaceVolume.PersistentVolumeClaim.ClaimName != "demo-city-workspace" {
		t.Fatalf("workspace claim = %q, want demo-city-workspace", workspaceVolume.PersistentVolumeClaim.ClaimName)
	}
	if _, ok := volumeMountByName(pod.Spec.Containers[0].VolumeMounts, "ws"); ok {
		t.Fatal("persistent workspace pod should not mount staged ws EmptyDir")
	}
	if _, ok := volumeByName(pod.Spec.Volumes, "city"); ok {
		t.Fatal("persistent workspace pod should not create compatibility city EmptyDir")
	}

	entrypoint := pod.Spec.Containers[0].Args[0]
	if strings.Contains(entrypoint, ".gc-workspace-ready") || strings.Contains(entrypoint, ".gc-ready") {
		t.Fatalf("persistent workspace entrypoint should not wait for staged workspace markers: %s", entrypoint)
	}
	if !strings.Contains(entrypoint, "cd '/workspace/cities/demo-city/.gc/worktrees/demo/cartographer' && tmux new-session") {
		t.Fatalf("entrypoint does not launch from persistent workdir: %s", entrypoint)
	}
	wantPreStartB64 := base64.StdEncoding.EncodeToString([]byte("test -d /workspace/cities/demo-city/rigs/demo"))
	if !strings.Contains(entrypoint, wantPreStartB64) {
		t.Fatalf("pre_start command was not remapped to persistent workspace root: %s", entrypoint)
	}
	wantCommandB64 := base64.StdEncoding.EncodeToString([]byte("gc agent-script --script /workspace/cities/demo-city/packs/demo/agent.yaml"))
	if !strings.Contains(entrypoint, wantCommandB64) {
		t.Fatalf("agent command was not remapped to persistent workspace root: %s", entrypoint)
	}

	envMap := map[string]string{}
	for _, e := range pod.Spec.Containers[0].Env {
		envMap[e.Name] = e.Value
	}
	want := map[string]string{
		"GC_CITY":             "/workspace/cities/demo-city",
		"GC_CITY_PATH":        "/workspace/cities/demo-city",
		"GC_DIR":              "/workspace/cities/demo-city/.gc/worktrees/demo/cartographer",
		"GC_RIG_ROOT":         "/workspace/cities/demo-city/rigs/demo",
		"GC_STORE_ROOT":       "/workspace/cities/demo-city/rigs/demo",
		"BEADS_DIR":           "/workspace/cities/demo-city/rigs/demo/.beads",
		"GC_CITY_RUNTIME_DIR": "/workspace/cities/demo-city/.gc/runtime",
	}
	for key, wantValue := range want {
		if got := envMap[key]; got != wantValue {
			t.Fatalf("%s = %q, want %q", key, got, wantValue)
		}
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
