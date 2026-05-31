package hybrid

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/runtime"
)

func isRemote(name string) bool { return strings.Contains(name, "polecat") }

func TestStart_RoutesToLocal(t *testing.T) {
	local, remote := runtime.NewFake(), runtime.NewFake()
	h := New(local, remote, isRemote)

	if err := h.Start(context.Background(), "refinery", runtime.Config{}); err != nil {
		t.Fatal(err)
	}
	if !local.IsRunning("refinery") {
		t.Error("expected local to have session")
	}
	if remote.IsRunning("refinery") {
		t.Error("remote should not have session")
	}
}

func TestStart_RoutesToRemote(t *testing.T) {
	local, remote := runtime.NewFake(), runtime.NewFake()
	h := New(local, remote, isRemote)

	if err := h.Start(context.Background(), "polecat-1", runtime.Config{}); err != nil {
		t.Fatal(err)
	}
	if local.IsRunning("polecat-1") {
		t.Error("local should not have session")
	}
	if !remote.IsRunning("polecat-1") {
		t.Error("expected remote to have session")
	}
}

func TestListRunning_MergesBothBackends(t *testing.T) {
	local, remote := runtime.NewFake(), runtime.NewFake()
	h := New(local, remote, isRemote)

	_ = h.Start(context.Background(), "gc-demo--refinery", runtime.Config{})
	_ = h.Start(context.Background(), "gc-demo--polecat-1", runtime.Config{})
	_ = h.Start(context.Background(), "gc-demo--polecat-2", runtime.Config{})

	names, err := h.ListRunning("gc-demo-")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 sessions, got %d: %v", len(names), names)
	}
}

func TestListRunning_PartialFailure(t *testing.T) {
	local := runtime.NewFake()
	remote := runtime.NewFailFake()
	h := New(local, remote, isRemote)

	_ = local.Start(context.Background(), "gc-demo--refinery", runtime.Config{})

	names, err := h.ListRunning("gc-demo-")
	if !runtime.IsPartialListError(err) {
		t.Fatalf("ListRunning error = %v, want partial list error", err)
	}
	if len(names) != 1 {
		t.Fatalf("expected 1 session from healthy backend, got %d", len(names))
	}
}

func TestListRunning_BothFail(t *testing.T) {
	local := runtime.NewFailFake()
	remote := runtime.NewFailFake()
	h := New(local, remote, isRemote)

	_, err := h.ListRunning("gc-demo-")
	if err == nil {
		t.Fatal("expected error when both backends fail")
	}
}

func TestAttach_RoutesCorrectly(t *testing.T) {
	local, remote := runtime.NewFake(), runtime.NewFake()
	h := New(local, remote, isRemote)

	_ = h.Start(context.Background(), "refinery", runtime.Config{})
	_ = h.Start(context.Background(), "polecat-1", runtime.Config{})

	if err := h.Attach("refinery"); err != nil {
		t.Errorf("attach local: %v", err)
	}
	if err := h.Attach("polecat-1"); err != nil {
		t.Errorf("attach remote: %v", err)
	}

	// Verify calls went to correct backends.
	var localAttach, remoteAttach int
	for _, c := range local.Calls {
		if c.Method == "Attach" {
			localAttach++
		}
	}
	for _, c := range remote.Calls {
		if c.Method == "Attach" {
			remoteAttach++
		}
	}
	if localAttach != 1 {
		t.Errorf("expected 1 local attach, got %d", localAttach)
	}
	if remoteAttach != 1 {
		t.Errorf("expected 1 remote attach, got %d", remoteAttach)
	}
}

func TestStop_RoutesCorrectly(t *testing.T) {
	local, remote := runtime.NewFake(), runtime.NewFake()
	h := New(local, remote, isRemote)

	_ = h.Start(context.Background(), "refinery", runtime.Config{})
	_ = h.Start(context.Background(), "polecat-1", runtime.Config{})

	if err := h.Stop("refinery"); err != nil {
		t.Fatal(err)
	}
	if err := h.Stop("polecat-1"); err != nil {
		t.Fatal(err)
	}

	if local.IsRunning("refinery") {
		t.Error("refinery should be stopped")
	}
	if remote.IsRunning("polecat-1") {
		t.Error("polecat-1 should be stopped")
	}
}

func TestPendingAndRespond_RouteToBackend(t *testing.T) {
	local, remote := runtime.NewFake(), runtime.NewFake()
	h := New(local, remote, isRemote)

	_ = h.Start(context.Background(), "polecat-1", runtime.Config{})
	remote.SetPendingInteraction("polecat-1", &runtime.PendingInteraction{RequestID: "req-1"})

	pending, err := h.Pending("polecat-1")
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if pending == nil || pending.RequestID != "req-1" {
		t.Fatalf("Pending = %#v, want req-1", pending)
	}
	if err := h.Respond("polecat-1", runtime.InteractionResponse{RequestID: "req-1", Action: "approve"}); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if got := remote.Responses["polecat-1"]; len(got) != 1 || got[0].Action != "approve" {
		t.Fatalf("Responses = %#v, want single approve", got)
	}
}

func TestPendingUnsupportedWhenBackendLacksInteractionSupport(t *testing.T) {
	local := &runtimeNoInteractionProvider{Provider: runtime.NewFake()}
	remote := runtime.NewFake()
	h := New(local, remote, isRemote)

	_, err := h.Pending("refinery")
	if !errors.Is(err, runtime.ErrInteractionUnsupported) {
		t.Fatalf("Pending error = %v, want ErrInteractionUnsupported", err)
	}
}

type runtimeNoInteractionProvider struct {
	runtime.Provider
}

type providerRuntimeCapabilityProvider struct {
	*runtime.Fake
	desired     runtime.ProviderRuntimeIdentity
	compat      runtime.CompatibilityObservation
	artifacts   []runtime.RuntimeArtifact
	desiredSeen []string
	compatSeen  []string
}

func (p *providerRuntimeCapabilityProvider) DesiredProviderRuntimeIdentity(_ context.Context, name string, _ runtime.Config) (runtime.ProviderRuntimeIdentity, error) {
	p.desiredSeen = append(p.desiredSeen, name)
	return p.desired, nil
}

func (p *providerRuntimeCapabilityProvider) ObserveRuntimeCompatibility(_ context.Context, name string, _ runtime.Config) (runtime.CompatibilityObservation, error) {
	p.compatSeen = append(p.compatSeen, name)
	return p.compat, nil
}

func (p *providerRuntimeCapabilityProvider) ListRuntimeArtifacts(prefix string) ([]runtime.RuntimeArtifact, error) {
	var out []runtime.RuntimeArtifact
	for _, artifact := range p.artifacts {
		if prefix == "" || strings.HasPrefix(artifact.Name, prefix) {
			out = append(out, artifact)
		}
	}
	return out, nil
}

func TestProviderRuntimeCapabilitiesDelegateToRoutedBackend(t *testing.T) {
	local := &providerRuntimeCapabilityProvider{Fake: runtime.NewFake()}
	remote := &providerRuntimeCapabilityProvider{
		Fake:    runtime.NewFake(),
		desired: runtime.ProviderRuntimeIdentity{Fingerprint: "remote-fingerprint", Version: "k8s-v3"},
		compat: runtime.CompatibilityObservation{
			Supported:  true,
			Exists:     true,
			Compatible: false,
			Reason:     "runtime-image-mismatch",
		},
	}
	h := New(local, remote, isRemote)

	desired, err := h.DesiredProviderRuntimeIdentity(context.Background(), "polecat-1", runtime.Config{})
	if err != nil {
		t.Fatalf("DesiredProviderRuntimeIdentity: %v", err)
	}
	if desired.Fingerprint != "remote-fingerprint" {
		t.Fatalf("desired identity = %#v, want remote fingerprint", desired)
	}
	compat, err := h.ObserveRuntimeCompatibility(context.Background(), "polecat-1", runtime.Config{})
	if err != nil {
		t.Fatalf("ObserveRuntimeCompatibility: %v", err)
	}
	if compat.Reason != "runtime-image-mismatch" {
		t.Fatalf("compat = %#v, want remote mismatch", compat)
	}
	if len(local.desiredSeen) != 0 || len(local.compatSeen) != 0 {
		t.Fatalf("local capability calls = desired %v compat %v, want none", local.desiredSeen, local.compatSeen)
	}
	if got := remote.desiredSeen; len(got) != 1 || got[0] != "polecat-1" {
		t.Fatalf("remote desired calls = %v, want [polecat-1]", got)
	}
	if got := remote.compatSeen; len(got) != 1 || got[0] != "polecat-1" {
		t.Fatalf("remote compat calls = %v, want [polecat-1]", got)
	}
}

func TestListRuntimeArtifactsMergesBothBackends(t *testing.T) {
	local := &providerRuntimeCapabilityProvider{
		Fake:      runtime.NewFake(),
		artifacts: []runtime.RuntimeArtifact{{Name: "gc-demo--refinery", SessionID: "local-session"}},
	}
	remote := &providerRuntimeCapabilityProvider{
		Fake:      runtime.NewFake(),
		artifacts: []runtime.RuntimeArtifact{{Name: "gc-demo--polecat-1", SessionID: "remote-session"}},
	}
	h := New(local, remote, isRemote)

	artifacts, err := h.ListRuntimeArtifacts("gc-demo-")
	if err != nil {
		t.Fatalf("ListRuntimeArtifacts: %v", err)
	}
	got := map[string]string{}
	for _, artifact := range artifacts {
		got[artifact.Name] = artifact.SessionID
	}
	if got["gc-demo--refinery"] != "local-session" || got["gc-demo--polecat-1"] != "remote-session" {
		t.Fatalf("artifacts = %#v, want local and remote artifacts", got)
	}
}

type deadRuntimeCheckProvider struct {
	*runtime.Fake
	dead   map[string]bool
	errs   map[string]error
	checks []string
}

func newDeadRuntimeCheckProvider() *deadRuntimeCheckProvider {
	return &deadRuntimeCheckProvider{
		Fake: runtime.NewFake(),
		dead: make(map[string]bool),
		errs: make(map[string]error),
	}
}

func (p *deadRuntimeCheckProvider) IsDeadRuntimeSession(name string) (bool, error) {
	p.checks = append(p.checks, name)
	if err := p.errs[name]; err != nil {
		return false, err
	}
	return p.dead[name], nil
}

func TestIsDeadRuntimeSessionDelegatesToRoutedChecker(t *testing.T) {
	local := newDeadRuntimeCheckProvider()
	remote := newDeadRuntimeCheckProvider()
	remote.dead["polecat-1"] = true
	h := New(local, remote, isRemote)

	dead, err := h.IsDeadRuntimeSession("polecat-1")
	if err != nil {
		t.Fatalf("IsDeadRuntimeSession: %v", err)
	}
	if !dead {
		t.Fatal("IsDeadRuntimeSession = false, want true from routed remote checker")
	}
	if len(local.checks) != 0 {
		t.Fatalf("local checks = %v, want none", local.checks)
	}
	if got := remote.checks; len(got) != 1 || got[0] != "polecat-1" {
		t.Fatalf("remote checks = %v, want [polecat-1]", got)
	}
}

func TestIsDeadRuntimeSessionReturnsFalseWhenRoutedBackendLacksChecker(t *testing.T) {
	local := runtime.NewFake()
	remote := newDeadRuntimeCheckProvider()
	remote.dead["refinery"] = true
	h := New(local, remote, isRemote)

	dead, err := h.IsDeadRuntimeSession("refinery")
	if err != nil {
		t.Fatalf("IsDeadRuntimeSession: %v", err)
	}
	if dead {
		t.Fatal("IsDeadRuntimeSession = true, want false for non-checker routed backend")
	}
	if len(remote.checks) != 0 {
		t.Fatalf("remote checks = %v, want none for local-routed session", remote.checks)
	}
}

func TestIsDeadRuntimeSessionReturnsRoutedCheckerError(t *testing.T) {
	local := newDeadRuntimeCheckProvider()
	remote := newDeadRuntimeCheckProvider()
	remote.errs["polecat-1"] = fmt.Errorf("runtime unavailable")
	h := New(local, remote, isRemote)

	dead, err := h.IsDeadRuntimeSession("polecat-1")
	if err == nil {
		t.Fatal("IsDeadRuntimeSession error = nil, want routed checker error")
	}
	if dead {
		t.Fatal("IsDeadRuntimeSession = true, want false on checker error")
	}
	if !strings.Contains(err.Error(), "runtime unavailable") {
		t.Fatalf("IsDeadRuntimeSession error = %v, want runtime unavailable", err)
	}
}
