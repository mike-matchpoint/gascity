package k8s

import (
	"fmt"
	"maps"
	"path/filepath"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gastownhall/gascity/internal/citylayout"
	"github.com/gastownhall/gascity/internal/pathutil"
	"github.com/gastownhall/gascity/internal/runtime"
	"github.com/gastownhall/gascity/internal/shellquote"
)

const (
	podManagedDoltHost = "dolt.gc.svc.cluster.local"
	podManagedDoltPort = "3307"

	defaultContainerHome    = "/home/gcagent"
	defaultPodWorkspaceRoot = "/workspace"
	podEntrypointWorkDir    = "/workspace"
	podLaunchDir            = "/gc-launch"
	podLaunchEntrypoint     = podLaunchDir + "/entrypoint.sh"
	podLaunchAgentScript    = podLaunchDir + "/agent-launch.sh"
	podLaunchPreStartDir    = podLaunchDir + "/pre-start"
	podLaunchPromptPath     = podLaunchDir + "/prompt.txt"
	podLaunchReadyMarker    = podLaunchDir + "/.gc-launch-ready"
	claudeSecretName        = "claude-credentials"
	codexSecretName         = "codex-credentials"
	gitSecretName           = "git-credentials"
)

type podLaunchMaterial struct {
	Entrypoint string
	Agent      string
	PreStart   []string
	Prompt     string
	HasPrompt  bool
}

func controllerCityPath(cfgEnv map[string]string) string {
	ctrlCity := strings.TrimSpace(cfgEnv["GC_CITY"])
	if ctrlCity == "" {
		ctrlCity = strings.TrimSpace(cfgEnv["GC_CITY_PATH"])
	}
	if ctrlCity == "" {
		ctrlCity = strings.TrimSpace(cfgEnv["GC_CITY_ROOT"])
	}
	return ctrlCity
}

func remapControllerPathToPodRoot(val, ctrlCity, podCityRoot string) string {
	val = strings.TrimSpace(val)
	ctrlCity = strings.TrimSpace(ctrlCity)
	podCityRoot = strings.TrimRight(strings.TrimSpace(podCityRoot), "/")
	if podCityRoot == "" {
		podCityRoot = defaultPodWorkspaceRoot
	}
	if val == "" || ctrlCity == "" {
		return val
	}
	if val == ctrlCity || strings.HasPrefix(val, ctrlCity+"/") {
		suffix := val[len(ctrlCity):]
		if podCityRoot == "/" {
			if suffix == "" {
				return "/"
			}
			return suffix
		}
		return podCityRoot + suffix
	}
	return val
}

func projectedPodWorkDir(cfg runtime.Config) string {
	return projectedPodWorkDirForControllerPath(cfg.WorkDir, controllerCityPath(cfg.Env))
}

func projectedPodWorkDirForProvider(cfg runtime.Config, p *Provider) string {
	ctrlCity := controllerCityPath(cfg.Env)
	return projectedPodWorkDirForControllerPathRoot(cfg.WorkDir, ctrlCity, p.podWorkspaceRoot())
}

func projectedPodStoreRoot(cfg runtime.Config, podWorkDir string) string {
	return projectedPodStoreRootForRoot(cfg, podWorkDir, defaultPodWorkspaceRoot)
}

func projectedPodStoreRootForRoot(cfg runtime.Config, podWorkDir, podCityRoot string) string {
	storeRoot := strings.TrimSpace(cfg.Env["GC_STORE_ROOT"])
	if storeRoot == "" {
		storeRoot = strings.TrimSpace(cfg.WorkDir)
	}
	if storeRoot == "" {
		storeRoot = controllerCityPath(cfg.Env)
	}
	storeRoot = remapControllerPathToPodRoot(storeRoot, controllerCityPath(cfg.Env), podCityRoot)
	if storeRoot == "" {
		return podWorkDir
	}
	return storeRoot
}

func projectedPodRuntimeDirForRoot(cfgEnv map[string]string, ctrlCity, podCityRoot string) string {
	podCity := strings.TrimRight(strings.TrimSpace(podCityRoot), "/")
	if podCity == "" {
		podCity = defaultPodWorkspaceRoot
	}
	runtimeDir := strings.TrimSpace(cfgEnv["GC_CITY_RUNTIME_DIR"])
	if runtimeDir == "" {
		return citylayout.RuntimeDataDir(podCity)
	}
	remapped := remapControllerPathToPodRoot(runtimeDir, ctrlCity, podCity)
	if remapped != runtimeDir {
		return remapped
	}
	return citylayout.RuntimeDataDir(podCity)
}

func projectControllerRuntimePathToPodRoot(path, ctrlCity, ctrlRuntimeDir, podRuntimeDir, podCityRoot string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if remapped := remapControllerPathToPodRoot(path, ctrlCity, podCityRoot); remapped != path {
		return remapped
	}
	if ctrlRuntimeDir != "" && pathutil.PathWithin(ctrlRuntimeDir, path) {
		normalizedRoot := pathutil.NormalizePathForCompare(ctrlRuntimeDir)
		normalizedPath := pathutil.NormalizePathForCompare(path)
		rel, err := filepath.Rel(normalizedRoot, normalizedPath)
		if err == nil {
			if rel == "." {
				return podRuntimeDir
			}
			return filepath.Join(podRuntimeDir, rel)
		}
	}
	return path
}

// projectedPodDoltEnv adapts the controller projection to a pod-visible Dolt
// target. Managed-local controller projections intentionally omit GC_DOLT_HOST
// and use a host-local runtime port; pods translate that blank-host managed
// shape to the provider-configured in-cluster alias at this adapter edge so
// agents still consume one GC_DOLT_* connection contract. Explicit
// GC_DOLT_HOST values are preserved as written.
// BEADS_DOLT_SERVER_HOST/PORT are compatibility mirrors derived from the GC
// projection, not independent input authorities.
func controllerLocalDoltHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	switch host {
	case "", "127.0.0.1", "localhost", "0.0.0.0", "::1", "::":
		return true
	default:
		return false
	}
}

func projectedPodDoltEnv(cfgEnv map[string]string, managedHost, managedPort string) (map[string]string, error) {
	host := strings.TrimSpace(cfgEnv["GC_DOLT_HOST"])
	port := strings.TrimSpace(cfgEnv["GC_DOLT_PORT"])
	managedHost = strings.TrimSpace(managedHost)
	managedPort = strings.TrimSpace(managedPort)
	if managedHost == "" {
		managedHost = podManagedDoltHost
	}
	if managedPort == "" {
		managedPort = podManagedDoltPort
	}

	switch {
	case host == "" && port == "":
		return map[string]string{}, nil
	case host != "" && port == "":
		return nil, fmt.Errorf("requires both GC_DOLT_HOST and GC_DOLT_PORT when GC_DOLT_HOST is set")
	case controllerLocalDoltHost(host):
		host = managedHost
		port = managedPort
	}

	projected := map[string]string{
		"GC_DOLT_HOST":           host,
		"GC_DOLT_PORT":           port,
		"BEADS_DOLT_SERVER_HOST": host,
		"BEADS_DOLT_SERVER_PORT": port,
	}
	return projected, nil
}

// buildPod creates a pod manifest compatible with gc-session-k8s.
// Same labels, annotations, container names, volumes, and tmux-inside-pod
// pattern so mixed-mode migration works.
func buildPod(name string, cfg runtime.Config, p *Provider) (*corev1.Pod, error) {
	podName := SanitizeName(name)
	label := SanitizeLabel(name)
	sessionKey := SessionKeyLabel(name)
	agentName := cfg.Env["GC_ALIAS"]
	if agentName == "" {
		agentName = cfg.Env["GC_AGENT"]
	}
	if agentName == "" {
		agentName = "unknown"
	}
	agentLabel := SanitizeLabel(agentName)

	// Resolve pod-side working directory.
	// Controller resolves dirs relative to its city path; pods use either the
	// staged workspace root or an explicitly mounted persistent workspace.
	podCityRoot := p.podWorkspaceRoot()
	podWorkDir := projectedPodWorkDirForProvider(cfg, p)
	ctrlCity := controllerCityPath(cfg.Env)

	// Dynamic user creation: when LINUX_USERNAME is set, the container starts
	// as root (see securityContext below), creates the user, sets up workspace
	// ownership, then drops privileges via su for the tmux session.
	linuxUsername := cfg.Env["LINUX_USERNAME"]
	credentialProfiles := podCredentialProfiles(cfg)

	// Build environment, remapping K8s-specific vars.
	var env []corev1.EnvVar
	var err error
	if len(cfg.ProviderCredentials) > 0 {
		env, err = buildPodEnvForRoot(cfg.Env, podCityRoot, podWorkDir, p.managedServiceHost, p.managedServicePort, credentialProfiles)
	} else {
		env, err = buildPodEnvForRoot(cfg.Env, podCityRoot, podWorkDir, p.managedServiceHost, p.managedServicePort)
	}
	if err != nil {
		return nil, err
	}
	env = appendAgentEnvDefaults(env, p.agentEnv)

	// Build volume mounts for the main container.
	// When prebaked, skip the ws EmptyDir — it would shadow baked image content.
	mainVolMounts := []corev1.VolumeMount{{
		Name: "launch", MountPath: podLaunchDir,
	}}
	volumes := []corev1.Volume{{
		Name: "launch", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}}

	if p.usesPersistentWorkspace() {
		mainVolMounts = append(mainVolMounts, corev1.VolumeMount{
			Name: "workspace", MountPath: podCityRoot,
		})
		volumes = append(volumes, corev1.Volume{
			Name: "workspace",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: p.workspacePVC,
				},
			},
		})
	} else if !p.prebaked {
		mainVolMounts = append(mainVolMounts, corev1.VolumeMount{
			Name: "ws", MountPath: "/workspace",
		})
		volumes = append(volumes, corev1.Volume{
			Name: "ws", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
	}

	for i, profile := range credentialProfiles {
		if strings.TrimSpace(profile.SecretName) == "" {
			continue
		}
		volumeName := credentialVolumeName(i, profile)
		mountPath := credentialMountPath(i, profile)
		mainVolMounts = append(mainVolMounts, corev1.VolumeMount{
			Name: volumeName, MountPath: mountPath, ReadOnly: true,
		})
		optional := profile.Optional
		volumes = append(volumes, corev1.Volume{
			Name: volumeName, VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: profile.SecretName,
					Optional:   boolPtr(optional),
				},
			},
		})
	}
	mainVolMounts = append(mainVolMounts, corev1.VolumeMount{
		Name: "git-credentials", MountPath: "/tmp/git-secret", ReadOnly: true,
	})
	volumes = append(volumes, corev1.Volume{
		Name: "git-credentials", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: gitSecretNameForEnv(cfg.Env),
				Optional:   boolPtr(true),
			},
		},
	})

	// If GC_CITY differs from work_dir, add a city volume (not needed when prebaked).
	if !p.prebaked && !p.usesPersistentWorkspace() && ctrlCity != "" && ctrlCity != cfg.WorkDir {
		mainVolMounts = append(mainVolMounts, corev1.VolumeMount{
			Name: "city", MountPath: ctrlCity,
		})
		volumes = append(volumes, corev1.Volume{
			Name:         "city",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
	}

	// Resources.
	resources, err := buildResources(p)
	if err != nil {
		return nil, err
	}
	runtimeIdentity, err := p.desiredProviderRuntimeIdentity(name, cfg)
	if err != nil {
		return nil, err
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: p.namespace,
			Labels: map[string]string{
				"app":            "gc-agent",
				"gc-session":     label,
				"gc-session-key": sessionKey,
				"gc-agent":       agentLabel,
			},
			Annotations: map[string]string{
				"gc-session-name":                           name,
				providerRuntimeFingerprintAnnotation:        runtimeIdentity.Fingerprint,
				providerRuntimeFingerprintVersionAnnotation: runtimeIdentity.Version,
				providerRuntimeImageAnnotation:              p.image,
				providerRuntimeProviderAnnotation:           "k8s",
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName:    podServiceAccount(name, cfg.Env, p),
			RestartPolicy:         corev1.RestartPolicyNever,
			ShareProcessNamespace: boolPtr(true),
			Containers: []corev1.Container{{
				Name:            "agent",
				Image:           p.image,
				ImagePullPolicy: corev1.PullAlways,
				WorkingDir:      podEntrypointWorkDir,
				Command:         []string{"/bin/sh", podLaunchEntrypoint},
				Env:             env,
				Stdin:           true,
				TTY:             true,
				Resources:       resources,
				VolumeMounts:    mainVolMounts,
				SecurityContext: agentSecurityContext(linuxUsername),
			}},
			Volumes: volumes,
		},
	}

	pod.Spec.InitContainers = []corev1.Container{{
		Name:            "launch",
		Image:           p.image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{
			"sh",
			"-c",
			fmt.Sprintf("while [ ! -f %s ]; do sleep 0.5; done", shellSingleQuote(podLaunchReadyMarker)),
		},
		VolumeMounts: []corev1.VolumeMount{{
			Name: "launch", MountPath: podLaunchDir,
		}},
		SecurityContext: agentSecurityContext(linuxUsername),
	}}

	// Apply optional scheduling fields.
	pod.Spec.NodeSelector = maps.Clone(p.nodeSelector)
	pod.Spec.Tolerations = cloneTolerations(p.tolerations)
	if p.affinity != nil {
		pod.Spec.Affinity = p.affinity.DeepCopy()
	}
	pod.Spec.TopologySpreadConstraints = cloneTopologySpreadConstraints(p.topologySpread)
	pod.Spec.PriorityClassName = p.priorityClassName

	// Add init container when staging is needed (skip when prebaked).
	if !p.prebaked && !p.usesPersistentWorkspace() && needsStaging(cfg, ctrlCity) {
		initVolMounts := []corev1.VolumeMount{
			{Name: "ws", MountPath: "/workspace"},
		}
		if ctrlCity != "" && ctrlCity != cfg.WorkDir {
			initVolMounts = append(initVolMounts, corev1.VolumeMount{
				Name: "city", MountPath: "/city-stage",
			})
		}
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
			Name:            "stage",
			Image:           p.image,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"sh", "-c", "while [ ! -f /workspace/.gc-ready ]; do sleep 0.5; done"},
			VolumeMounts:    initVolMounts,
		})
	}

	return pod, nil
}

func cloneTolerations(in []corev1.Toleration) []corev1.Toleration {
	if len(in) == 0 {
		return nil
	}
	out := append([]corev1.Toleration(nil), in...)
	for i := range out {
		if in[i].TolerationSeconds != nil {
			seconds := *in[i].TolerationSeconds
			out[i].TolerationSeconds = &seconds
		}
	}
	return out
}

func cloneTopologySpreadConstraints(in []corev1.TopologySpreadConstraint) []corev1.TopologySpreadConstraint {
	if len(in) == 0 {
		return nil
	}
	out := make([]corev1.TopologySpreadConstraint, len(in))
	for i := range in {
		out[i] = *in[i].DeepCopy()
	}
	return out
}

// agentSecurityContext returns a container security context.
// When a dynamic linux username is configured, the container starts as root
// (UID 0) so it can create the user at runtime before dropping privileges.
// When no dynamic user is set, returns nil (uses Dockerfile default: gcagent).
func agentSecurityContext(linuxUsername string) *corev1.SecurityContext {
	if linuxUsername == "" {
		return nil
	}
	var rootUID int64
	return &corev1.SecurityContext{
		RunAsUser: &rootUID,
	}
}

// buildPodEnv creates the env var list for the agent container.
// Removes controller-only vars, strips deprecated K8s compatibility inputs,
// and remaps pod-visible ones.
func buildPodEnv(cfgEnv map[string]string, podWorkDir, managedServiceHost, managedServicePort string) ([]corev1.EnvVar, error) {
	return buildPodEnvForRoot(cfgEnv, defaultPodWorkspaceRoot, podWorkDir, managedServiceHost, managedServicePort)
}

func buildPodEnvForRoot(cfgEnv map[string]string, podCityRoot, podWorkDir, managedServiceHost, managedServicePort string, profileSets ...[]runtime.ProviderCredentialProfile) ([]corev1.EnvVar, error) {
	var profiles []runtime.ProviderCredentialProfile
	if len(profileSets) > 0 {
		profiles = profileSets[0]
	}
	// Start with cfg.Env, removing controller-only vars.
	// Auth creds (GC_DOLT_USER, GC_DOLT_PASSWORD, BEADS_DOLT_*_USER/PASSWORD) intentionally pass through.
	skip := map[string]bool{
		"GC_BEADS":                               true,
		"GC_SESSION":                             true,
		"GC_EVENTS":                              true,
		"GC_K8S_DOLT_HOST":                       true,
		"GC_K8S_DOLT_PORT":                       true,
		"GC_K8S_AGENT_ENV_JSON":                  true,
		"GC_DOLT_HOST":                           true,
		"GC_DOLT_PORT":                           true,
		"BEADS_DOLT_SERVER_HOST":                 true,
		"BEADS_DOLT_SERVER_PORT":                 true,
		"AWS_ACCESS_KEY_ID":                      true,
		"AWS_SECRET_ACCESS_KEY":                  true,
		"AWS_SESSION_TOKEN":                      true,
		"AWS_SECURITY_TOKEN":                     true,
		"AWS_PROFILE":                            true,
		"AWS_ROLE_ARN":                           true,
		"AWS_WEB_IDENTITY_TOKEN_FILE":            true,
		"AWS_CONTAINER_CREDENTIALS_FULL_URI":     true,
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": true,
		"AWS_CONTAINER_AUTHORIZATION_TOKEN":      true,
		"AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE": true,
		"HOME":                                   true,
		"CODEX_HOME":                             true,
		"CLAUDE_CONFIG_DIR":                      true,
		"XDG_CONFIG_HOME":                        true,
		"XDG_STATE_HOME":                         true,
	}

	ctrlCity := controllerCityPath(cfgEnv)
	ctrlRuntimeDir := strings.TrimSpace(cfgEnv["GC_CITY_RUNTIME_DIR"])
	podCityRoot = strings.TrimRight(strings.TrimSpace(podCityRoot), "/")
	if podCityRoot == "" {
		podCityRoot = defaultPodWorkspaceRoot
	}
	podRuntimeDir := projectedPodRuntimeDirForRoot(cfgEnv, ctrlCity, podCityRoot)

	var env []corev1.EnvVar
	for k, v := range cfgEnv {
		if skip[k] {
			continue
		}
		val := v
		// Remap city/workdir vars to pod-visible paths.
		switch k {
		case "GC_CITY", "GC_CITY_PATH", "GC_CITY_ROOT":
			val = podCityRoot
		case "GC_DIR":
			val = podWorkDir
		case "GC_CITY_RUNTIME_DIR":
			val = podRuntimeDir
		case "GC_CONTROL_DISPATCHER_TRACE_DEFAULT", "GC_PACK_STATE_DIR":
			val = projectControllerRuntimePathToPodRoot(val, ctrlCity, ctrlRuntimeDir, podRuntimeDir, podCityRoot)
		case "GC_STORE_ROOT", "GC_RIG_ROOT", "BEADS_DIR", "GT_ROOT", "GC_PACK_DIR":
			val = remapControllerPathToPodRoot(val, ctrlCity, podCityRoot)
		}
		env = append(env, corev1.EnvVar{Name: k, Value: val})
	}

	projectedDolt, err := projectedPodDoltEnv(cfgEnv, managedServiceHost, managedServicePort)
	if err != nil {
		return nil, err
	}
	projectedKeys := make([]string, 0, len(projectedDolt))
	for key := range projectedDolt {
		projectedKeys = append(projectedKeys, key)
	}
	sort.Strings(projectedKeys)
	for _, key := range projectedKeys {
		env = append(env, corev1.EnvVar{Name: key, Value: projectedDolt[key]})
	}

	// Add tmux session env so agent's tmux provider uses the same session.
	env = append(env, corev1.EnvVar{Name: "GC_TMUX_SESSION", Value: tmuxSession})

	providerHome := podProviderHome(cfgEnv)
	configuredProfiles := len(profileSets) > 0
	profiles = normalizeCredentialProfiles(profiles, providerHome)
	for _, profile := range profiles {
		keys := make([]string, 0, len(profile.Env))
		for key := range profile.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			env = append(env, corev1.EnvVar{Name: key, Value: expandProviderCredentialValue(profile.Env[key], profile, providerHome)})
		}
		for _, secretEnv := range profile.EnvFromSecret {
			if secretEnv.Name == "" || secretEnv.Key == "" {
				continue
			}
			secretName := strings.TrimSpace(secretEnv.SecretName)
			if secretName == "" {
				secretName = profile.SecretName
			}
			if secretName == "" {
				continue
			}
			env = append(env, credentialSecretEnv(secretEnv.Name, secretName, secretEnv.Key, secretEnv.Optional))
		}
	}

	// Inject GITHUB_TOKEN from optional K8s secret for git push in pods.
	gitSecretName := gitSecretNameForEnv(cfgEnv)
	env = append(env, gitTokenEnv("GITHUB_TOKEN", gitSecretName))
	env = append(env, gitTokenEnv("GH_TOKEN", gitSecretName))
	if !configuredProfiles && strings.TrimSpace(cfgEnv["CLAUDE_CODE_OAUTH_TOKEN"]) == "" {
		env = append(env, claudeOAuthTokenEnv(claudeSecretName))
	}

	return env, nil
}

func appendAgentEnvDefaults(env []corev1.EnvVar, defaults map[string]string) []corev1.EnvVar {
	if len(defaults) == 0 {
		return env
	}
	present := make(map[string]bool, len(env))
	for _, entry := range env {
		present[entry.Name] = true
	}
	keys := make([]string, 0, len(defaults))
	for key := range defaults {
		key = strings.TrimSpace(key)
		if key == "" || present[key] {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, corev1.EnvVar{Name: key, Value: defaults[key]})
		present[key] = true
	}
	return env
}

func podProviderHome(cfgEnv map[string]string) string {
	if linuxUser := strings.TrimSpace(cfgEnv["LINUX_USERNAME"]); linuxUser != "" {
		return "/home/" + linuxUser
	}
	if home := strings.TrimSpace(cfgEnv["GC_K8S_CONTAINER_HOME"]); home != "" {
		return strings.TrimRight(home, "/")
	}
	return defaultContainerHome
}

func podServiceAccount(sessionName string, cfgEnv map[string]string, p *Provider) string {
	explicitServiceAccount := strings.TrimSpace(cfgEnv["GC_K8S_SERVICE_ACCOUNT"])
	fallbackServiceAccount := strings.TrimSpace(p.serviceAccount)
	if explicitServiceAccount != "" && (len(p.serviceAccountMap) == 0 || explicitServiceAccount != fallbackServiceAccount) {
		return explicitServiceAccount
	}
	if serviceAccount := mappedPodServiceAccount(sessionName, cfgEnv, p.serviceAccountMap); serviceAccount != "" {
		return serviceAccount
	}
	if explicitServiceAccount != "" {
		return explicitServiceAccount
	}
	return p.serviceAccount
}

func mappedPodServiceAccount(sessionName string, cfgEnv map[string]string, serviceAccounts map[string]string) string {
	if len(serviceAccounts) == 0 {
		return ""
	}
	for _, key := range podServiceAccountKeys(sessionName, cfgEnv) {
		if serviceAccount := strings.TrimSpace(serviceAccounts[key]); serviceAccount != "" {
			return serviceAccount
		}
	}
	return ""
}

func podServiceAccountKeys(sessionName string, cfgEnv map[string]string) []string {
	var keys []string
	keys = appendPodServiceAccountKeyCandidates(keys, cfgEnv["GC_K8S_SERVICE_ACCOUNT_KEY"])
	keys = appendPodServiceAccountKeyCandidates(keys, cfgEnv["GC_TEMPLATE"])
	keys = appendPodServiceAccountKeyCandidates(keys, cfgEnv["GC_ALIAS"])
	keys = appendPodServiceAccountKeyCandidates(keys, cfgEnv["GC_AGENT"])
	keys = appendPodServiceAccountKeyCandidates(keys, sessionName)
	return keys
}

func appendPodServiceAccountKeyCandidates(keys []string, identity string) []string {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return keys
	}
	keys = appendUniqueString(keys, identity)
	if roleKey := podServiceAccountRoleKey(identity); roleKey != "" {
		keys = appendUniqueString(keys, roleKey)
	}
	return keys
}

func podServiceAccountRoleKey(identity string) string {
	key := strings.TrimSpace(identity)
	if key == "" {
		return ""
	}
	if idx := strings.LastIndex(key, "/"); idx >= 0 {
		key = key[idx+1:]
	}
	if idx := strings.LastIndex(key, "."); idx >= 0 {
		key = key[idx+1:]
	}
	for _, marker := range []string{"-vgc-session-", "-vgemcd-session-", "-session-"} {
		if idx := strings.Index(key, marker); idx > 0 {
			key = key[:idx]
			break
		}
	}
	return strings.TrimSpace(key)
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func gitSecretNameForEnv(cfgEnv map[string]string) string {
	if secretName := strings.TrimSpace(cfgEnv["GC_K8S_GIT_SECRET_NAME"]); secretName != "" {
		return secretName
	}
	return gitSecretName
}

func podCredentialProfiles(cfg runtime.Config) []runtime.ProviderCredentialProfile {
	return normalizeCredentialProfiles(cfg.ProviderCredentials, podProviderHome(cfg.Env))
}

func normalizeCredentialProfiles(profiles []runtime.ProviderCredentialProfile, providerHome string) []runtime.ProviderCredentialProfile {
	if len(profiles) == 0 {
		return legacyCredentialProfiles(providerHome)
	}
	out := make([]runtime.ProviderCredentialProfile, 0, len(profiles))
	for i, profile := range profiles {
		normalized := profile
		if strings.TrimSpace(normalized.Name) == "" {
			normalized.Name = strings.TrimSpace(normalized.SecretName)
		}
		if strings.TrimSpace(normalized.Name) == "" {
			normalized.Name = fmt.Sprintf("provider-credentials-%d", i)
		}
		if strings.TrimSpace(normalized.MountPath) == "" {
			normalized.MountPath = "/tmp/gc-provider-secrets/" + credentialVolumeName(i, normalized)
		}
		normalized.Env = maps.Clone(normalized.Env)
		if normalized.EnvFromSecret != nil {
			normalized.EnvFromSecret = append([]runtime.ProviderSecretEnv(nil), normalized.EnvFromSecret...)
		}
		if normalized.Copy != nil {
			normalized.Copy = append([]runtime.ProviderCredentialCopy(nil), normalized.Copy...)
		}
		out = append(out, normalized)
	}
	return out
}

func legacyCredentialProfiles(providerHome string) []runtime.ProviderCredentialProfile {
	return []runtime.ProviderCredentialProfile{
		{
			Name:       "claude-config",
			SecretName: claudeSecretName,
			MountPath:  "/tmp/claude-secret",
			TargetDir:  ".claude",
			Optional:   true,
			Env:        map[string]string{"CLAUDE_CONFIG_DIR": filepath.Join(providerHome, ".claude")},
			Copy: []runtime.ProviderCredentialCopy{{
				Source: ".claude.json",
				Target: ".claude.json",
			}},
		},
		{
			Name:       "codex-config",
			SecretName: codexSecretName,
			MountPath:  "/tmp/codex-secret",
			TargetDir:  ".codex",
			Optional:   true,
			Env:        map[string]string{"CODEX_HOME": filepath.Join(providerHome, ".codex")},
		},
	}
}

func credentialVolumeName(index int, profile runtime.ProviderCredentialProfile) string {
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = strings.TrimSpace(profile.SecretName)
	}
	if name == "" {
		name = fmt.Sprintf("provider-credentials-%d", index)
	}
	return SanitizeName(name)
}

func credentialMountPath(index int, profile runtime.ProviderCredentialProfile) string {
	if mountPath := strings.TrimSpace(profile.MountPath); mountPath != "" {
		return mountPath
	}
	return "/tmp/gc-provider-secrets/" + credentialVolumeName(index, profile)
}

func providerCredentialPath(providerHome, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "/") {
		return filepath.Clean(path)
	}
	return filepath.Join(providerHome, path)
}

func expandProviderCredentialValue(value string, profile runtime.ProviderCredentialProfile, providerHome string) string {
	targetDir := providerCredentialPath(providerHome, profile.TargetDir)
	replacer := strings.NewReplacer(
		"{{.Home}}", providerHome,
		"{{ .Home }}", providerHome,
		"{{.TargetDir}}", targetDir,
		"{{ .TargetDir }}", targetDir,
	)
	return replacer.Replace(value)
}

func gitTokenEnv(name, secretName string) corev1.EnvVar {
	return credentialSecretEnv(name, secretName, "token", true)
}

func claudeOAuthTokenEnv(secretName string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: "CLAUDE_CODE_OAUTH_TOKEN",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  "CLAUDE_CODE_OAUTH_TOKEN",
				Optional:             boolPtr(true),
			},
		},
	}
}

func credentialSecretEnv(name, secretName, key string, optional bool) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  key,
				Optional:             boolPtr(optional),
			},
		},
	}
}

// needsStaging returns true if the session config requires file staging
// via init container.
func needsStaging(cfg runtime.Config, ctrlCity string) bool {
	if cfg.OverlayDir != "" {
		return true
	}
	if len(cfg.PackOverlayDirs) > 0 {
		return true
	}
	if len(cfg.CopyFiles) > 0 {
		return true
	}
	if needsCityRootRuntimeInputStaging(cfg.WorkDir, ctrlCity) {
		return true
	}
	// Rig agents have a work_dir subdirectory.
	if cfg.WorkDir != "" && cfg.WorkDir != ctrlCity {
		return true
	}
	return false
}

// buildResources creates resource requirements from the provider config.
// Returns an error if any resource quantity string is invalid, instead of
// panicking via MustParse.
func buildResources(p *Provider) (corev1.ResourceRequirements, error) {
	req := corev1.ResourceRequirements{}
	if p.cpuRequest != "" || p.memRequest != "" {
		req.Requests = corev1.ResourceList{}
		if p.cpuRequest != "" {
			q, err := resource.ParseQuantity(p.cpuRequest)
			if err != nil {
				return req, fmt.Errorf("parsing GC_K8S_CPU_REQUEST %q: %w", p.cpuRequest, err)
			}
			req.Requests[corev1.ResourceCPU] = q
		}
		if p.memRequest != "" {
			q, err := resource.ParseQuantity(p.memRequest)
			if err != nil {
				return req, fmt.Errorf("parsing GC_K8S_MEM_REQUEST %q: %w", p.memRequest, err)
			}
			req.Requests[corev1.ResourceMemory] = q
		}
	}
	if p.cpuLimit != "" || p.memLimit != "" {
		req.Limits = corev1.ResourceList{}
		if p.cpuLimit != "" {
			q, err := resource.ParseQuantity(p.cpuLimit)
			if err != nil {
				return req, fmt.Errorf("parsing GC_K8S_CPU_LIMIT %q: %w", p.cpuLimit, err)
			}
			req.Limits[corev1.ResourceCPU] = q
		}
		if p.memLimit != "" {
			q, err := resource.ParseQuantity(p.memLimit)
			if err != nil {
				return req, fmt.Errorf("parsing GC_K8S_MEM_LIMIT %q: %w", p.memLimit, err)
			}
			req.Limits[corev1.ResourceMemory] = q
		}
	}
	return req, nil
}

func boolPtr(b bool) *bool { return &b }

func buildPodLaunchMaterial(cfg runtime.Config, p *Provider) podLaunchMaterial {
	podCityRoot := p.podWorkspaceRoot()
	podWorkDir := projectedPodWorkDirForProvider(cfg, p)
	ctrlCity := controllerCityPath(cfg.Env)
	command := cfg.Command
	if command == "" {
		command = "/bin/bash"
	}
	if ctrlCity != "" {
		command = strings.ReplaceAll(command, ctrlCity, podCityRoot)
	}

	preStart := make([]string, 0, len(cfg.PreStart))
	for _, cmd := range cfg.PreStart {
		c := cmd
		if ctrlCity != "" {
			c = strings.ReplaceAll(c, ctrlCity, podCityRoot)
		}
		preStart = append(preStart, c)
	}

	prompt, hasPrompt := podPromptText(cfg)
	agent := podAgentLaunchScript(command, cfg.PromptFlag, hasPrompt)
	entrypoint := podEntrypointScript(cfg, p, podWorkDir)
	return podLaunchMaterial{
		Entrypoint: entrypoint,
		Agent:      agent,
		PreStart:   preStart,
		Prompt:     prompt,
		HasPrompt:  hasPrompt,
	}
}

func podEntrypointScript(cfg runtime.Config, p *Provider, podWorkDir string) string {
	linuxUsername := strings.TrimSpace(cfg.Env["LINUX_USERNAME"])
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	if linuxUsername != "" {
		b.WriteString(fmt.Sprintf(
			`id %s >/dev/null 2>&1 || useradd -m -s /bin/bash %s; `+"\n"+
				`echo %s > /etc/sudoers.d/%s && chmod 0440 /etc/sudoers.d/%s; `+"\n"+
				`mkdir -p %s && chown -R %s %s; `+"\n"+
				`chown -R %s %s 2>/dev/null || true; `+"\n"+
				`export HOME=%s USER=%s LOGNAME=%s SHELL="/bin/bash"; `+"\n",
			shellSingleQuote(linuxUsername), shellSingleQuote(linuxUsername),
			shellSingleQuote(linuxUsername+" ALL=(ALL) NOPASSWD:ALL"), shellSingleQuote(linuxUsername), shellSingleQuote(linuxUsername),
			shellSingleQuote(podWorkDir), shellSingleQuote(linuxUsername), shellSingleQuote(podWorkDir),
			shellSingleQuote(linuxUsername), shellSingleQuote(podLaunchDir),
			shellSingleQuote("/home/"+linuxUsername), shellSingleQuote(linuxUsername), shellSingleQuote(linuxUsername),
		))
	}
	for i, profile := range podCredentialProfiles(cfg) {
		mountPath := credentialMountPath(i, profile)
		if targetDir := providerCredentialPath(podProviderHome(cfg.Env), profile.TargetDir); targetDir != "" {
			b.WriteString(fmt.Sprintf(
				`mkdir -p %s && cp -rL %s/. %s/ 2>/dev/null || true; `+"\n",
				shellSingleQuote(targetDir),
				shellSingleQuote(mountPath),
				shellSingleQuote(targetDir),
			))
		}
		for _, cp := range profile.Copy {
			source := strings.TrimSpace(cp.Source)
			target := providerCredentialPath(podProviderHome(cfg.Env), cp.Target)
			if source == "" || target == "" {
				continue
			}
			b.WriteString(fmt.Sprintf(
				`mkdir -p %s && cp -f %s %s 2>/dev/null || true; `+"\n",
				shellSingleQuote(filepath.Dir(target)),
				shellSingleQuote(filepath.Join(mountPath, source)),
				shellSingleQuote(target),
			))
		}
	}
	b.WriteString(`git config --global --add safe.directory '*' 2>/dev/null || true; ` + "\n")
	b.WriteString(`if [ -n "${GITHUB_TOKEN:-}" ]; then export GH_TOKEN="${GH_TOKEN:-$GITHUB_TOKEN}"; gh auth setup-git >/dev/null 2>&1 || true; fi; ` + "\n")
	if !p.prebaked && !p.usesPersistentWorkspace() {
		b.WriteString(`while [ ! -f /workspace/.gc-workspace-ready ] && [ ! -f /workspace/.gc-ready ]; do sleep 0.5; done; ` + "\n")
	}
	b.WriteString(fmt.Sprintf(
		`for __gc_pre_start in %s/*.sh; do [ -f "$__gc_pre_start" ] || continue; sh "$__gc_pre_start"; done`+"\n",
		shellSingleQuote(podLaunchPreStartDir),
	))
	launchCommand := fmt.Sprintf(
		"cd %s && tmux new-session -d -s %s %s && sleep infinity",
		shellSingleQuote(podWorkDir),
		shellSingleQuote(tmuxSession),
		shellSingleQuote("sh "+podLaunchAgentScript),
	)
	if linuxUsername != "" {
		b.WriteString(fmt.Sprintf("su -m %s -c %s\n", shellSingleQuote(linuxUsername), shellSingleQuote(launchCommand)))
	} else {
		b.WriteString(launchCommand + "\n")
	}
	return b.String()
}

func podAgentLaunchScript(command, promptFlag string, hasPrompt bool) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	if hasPrompt {
		b.WriteString(fmt.Sprintf(
			`__gc_prompt="$(cat %s && printf .)"`+"\n"+
				`__gc_status=$?`+"\n"+
				`rm -f %s`+"\n"+
				`[ "$__gc_status" -eq 0 ] || exit "$__gc_status"`+"\n"+
				`__gc_prompt="${__gc_prompt%%.}"`+"\n",
			shellSingleQuote(podLaunchPromptPath),
			shellSingleQuote(podLaunchPromptPath),
		))
		promptArg := `"$1"`
		if strings.TrimSpace(promptFlag) != "" {
			promptArg = promptFlag + " " + promptArg
		}
		b.WriteString(fmt.Sprintf("exec sh -c %s sh \"$__gc_prompt\"\n", shellSingleQuote(command+" "+promptArg)))
	} else {
		b.WriteString(fmt.Sprintf("exec sh -c %s\n", shellSingleQuote(command)))
	}
	return b.String()
}

func podPromptText(cfg runtime.Config) (string, bool) {
	if strings.TrimSpace(cfg.PromptSuffix) == "" {
		return "", false
	}
	prompt := cfg.PromptSuffix
	if parts := shellquote.Split(cfg.PromptSuffix); len(parts) > 0 {
		prompt = parts[0]
	}
	return prompt, true
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
