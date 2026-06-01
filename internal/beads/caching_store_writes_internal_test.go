package beads

import (
	"context"
	"errors"
	"testing"
)

// countingBackingStore wraps a Store and counts SetMetadata /
// SetMetadataBatch / Update / Close invocations so tests can assert when
// CachingStore short-circuits a no-op write before the backing call.
type countingBackingStore struct {
	Store
	setMetadataCalls      int
	setMetadataBatchCalls int
	updateCalls           int
	closeCalls            int
	closeAllCalls         int
	closeAllIDs           []string
}

func (c *countingBackingStore) SetMetadata(id, key, value string) error {
	c.setMetadataCalls++
	return c.Store.SetMetadata(id, key, value)
}

func (c *countingBackingStore) SetMetadataBatch(id string, kvs map[string]string) error {
	c.setMetadataBatchCalls++
	return c.Store.SetMetadataBatch(id, kvs)
}

func (c *countingBackingStore) Update(id string, opts UpdateOpts) error {
	c.updateCalls++
	return c.Store.Update(id, opts)
}

func (c *countingBackingStore) Close(id string) error {
	c.closeCalls++
	return c.Store.Close(id)
}

func (c *countingBackingStore) CloseAll(ids []string, metadata map[string]string) (int, error) {
	c.closeAllCalls++
	c.closeAllIDs = append([]string(nil), ids...)
	return c.Store.CloseAll(ids, metadata)
}

type txPreservingBackingStore struct {
	Store
	txCalls     int
	updateCalls int
}

type partialRuntimeCloseStore struct {
	Store
}

type sparseRuntimeCreateStore struct {
	*MemStore
}

func (s *sparseRuntimeCreateStore) RuntimeCreate(ctx context.Context, b Bead, policy WritePolicy) (Bead, error) {
	created, err := s.MemStore.RuntimeCreate(ctx, b, policy)
	if err != nil {
		return Bead{}, err
	}
	return Bead{
		ID:        created.ID,
		Title:     created.Title,
		Status:    created.Status,
		Type:      created.Type,
		CreatedAt: created.CreatedAt,
	}, nil
}

func (s partialRuntimeCloseStore) RuntimeCreate(ctx context.Context, b Bead, policy WritePolicy) (Bead, error) {
	return RuntimeCreate(ctx, s.Store, b, policy)
}

func (s partialRuntimeCloseStore) RuntimeUpdate(ctx context.Context, id string, opts UpdateOpts, policy WritePolicy) error {
	return RuntimeUpdate(ctx, s.Store, id, opts, policy)
}

func (s partialRuntimeCloseStore) RuntimeCloseAll(_ context.Context, ids []string, metadata map[string]string, _ WritePolicy) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if _, err := s.CloseAll(ids[:1], metadata); err != nil {
		return 0, err
	}
	return 1, errors.New("second close failed")
}

func (s partialRuntimeCloseStore) RuntimePing(ctx context.Context, policy WritePolicy) error {
	return RuntimePing(ctx, s.Store, policy)
}

func TestCachingStoreRuntimeCreateSeedsSparseCreateOutput(t *testing.T) {
	backing := &sparseRuntimeCreateStore{MemStore: NewMemStore()}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	policy := RuntimeWritePolicy(WriteClassReservation, "test.runtime-create", "order-1")
	created, err := cache.RuntimeCreate(context.Background(), Bead{
		ID:          "gc-order-1",
		Title:       "order run",
		Type:        "task",
		Description: "dispatch",
		Assignee:    "worker",
		From:        "controller",
		Labels:      []string{"order-run:nightly", "order-tracking"},
		Metadata:    map[string]string{"gc.order.scoped": "nightly"},
	}, policy)
	if err != nil {
		t.Fatalf("RuntimeCreate: %v", err)
	}
	if !hasString(created.Labels, "order-run:nightly") || created.Metadata["gc.order.scoped"] != "nightly" {
		t.Fatalf("created bead lost seed labels/metadata: %#v", created)
	}

	got, err := cache.RuntimeGet(context.Background(), created.ID, RuntimeReadPolicy(ReadClassHotAuthoritative, "test.runtime-get"))
	if err != nil {
		t.Fatalf("RuntimeGet: %v", err)
	}
	if !hasString(got.Labels, "order-run:nightly") || !hasString(got.Labels, "order-tracking") {
		t.Fatalf("RuntimeGet labels = %#v, want seeded order labels", got.Labels)
	}
	if got.Metadata["gc.order.scoped"] != "nightly" || got.Metadata["from"] != "controller" {
		t.Fatalf("RuntimeGet metadata = %#v, want seeded order metadata and from", got.Metadata)
	}

	rows, err := cache.RuntimeList(context.Background(), ListQuery{Status: "open", AllowScan: true}, RuntimeReadPolicy(ReadClassHotAuthoritative, "test.runtime-list"))
	if err != nil {
		t.Fatalf("RuntimeList: %v", err)
	}
	if len(rows) != 1 || !hasString(rows[0].Labels, "order-run:nightly") {
		t.Fatalf("RuntimeList rows = %#v, want cached row with seeded label", rows)
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (s *txPreservingBackingStore) Update(id string, opts UpdateOpts) error {
	s.updateCalls++
	if err := s.Store.Update(id, opts); err != nil {
		return err
	}
	if opts.Title == nil {
		clobbered := ""
		return s.Store.Update(id, UpdateOpts{Title: &clobbered})
	}
	return nil
}

func (s *txPreservingBackingStore) Tx(commitMsg string, fn func(Tx) error) error {
	s.txCalls++
	return s.Store.Tx(commitMsg, fn)
}

func TestCachingStoreTxDelegatesToBackingTxAndRefreshesCache(t *testing.T) {
	t.Parallel()

	backing := &txPreservingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{
		Title:       "preserve title",
		Description: "before",
		Labels:      []string{"keep-label", "drop-label"},
		Metadata:    map[string]string{"existing": "yes"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	description := "after"
	if err := cache.Tx("preserve backing semantics", func(tx Tx) error {
		if err := tx.Update(bead.ID, UpdateOpts{
			Description:  &description,
			Labels:       []string{"new-label"},
			RemoveLabels: []string{"drop-label"},
		}); err != nil {
			return err
		}
		if err := tx.SetMetadataBatch(bead.ID, map[string]string{"tx": "applied"}); err != nil {
			return err
		}
		return tx.Close(bead.ID)
	}); err != nil {
		t.Fatalf("Tx: %v", err)
	}

	if backing.txCalls != 1 {
		t.Fatalf("backing.Tx calls = %d, want 1", backing.txCalls)
	}
	if backing.updateCalls != 0 {
		t.Fatalf("backing.Update calls = %d, want 0 direct calls through CachingStore", backing.updateCalls)
	}

	got, err := backing.Get(bead.ID)
	if err != nil {
		t.Fatalf("backing Get: %v", err)
	}
	assertTxPreservedBead(t, got)

	cached, err := cache.Get(bead.ID)
	if err != nil {
		t.Fatalf("cache Get: %v", err)
	}
	assertTxPreservedBead(t, cached)
}

func assertTxPreservedBead(t *testing.T, got Bead) {
	t.Helper()
	if got.Title != "preserve title" {
		t.Fatalf("Title = %q, want preserved title", got.Title)
	}
	if got.Description != "after" {
		t.Fatalf("Description = %q, want after", got.Description)
	}
	if got.Status != "closed" {
		t.Fatalf("Status = %q, want closed", got.Status)
	}
	if got.Metadata["existing"] != "yes" || got.Metadata["tx"] != "applied" {
		t.Fatalf("Metadata = %#v, want existing=yes and tx=applied", got.Metadata)
	}
	if !stringSliceContains(got.Labels, "keep-label") || !stringSliceContains(got.Labels, "new-label") || stringSliceContains(got.Labels, "drop-label") {
		t.Fatalf("Labels = %#v, want keep-label and new-label without drop-label", got.Labels)
	}
}

func TestCachingStoreRuntimeUpdateRefreshesHotGetCache(t *testing.T) {
	t.Parallel()

	backing := NewMemStore()
	bead, err := backing.Create(Bead{
		Title:    "worker",
		Type:     "session",
		Metadata: map[string]string{"state": "creating"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.PrimeActive(); err != nil {
		t.Fatalf("PrimeActive: %v", err)
	}

	policy := RuntimeWritePolicy(WriteClassHotState, "test.cache-runtime-update", "session:"+bead.ID)
	if err := RuntimeUpdate(context.Background(), cache, bead.ID, UpdateOpts{
		Metadata: map[string]string{"state": "active"},
	}, policy); err != nil {
		t.Fatalf("RuntimeUpdate: %v", err)
	}

	got, err := RuntimeGet(context.Background(), cache, bead.ID,
		RuntimeReadPolicy(ReadClassHotAuthoritative, "test.cache-runtime-get"))
	if err != nil {
		t.Fatalf("RuntimeGet: %v", err)
	}
	if got.Metadata["state"] != "active" {
		t.Fatalf("cached state = %q, want active", got.Metadata["state"])
	}
}

func TestCachingStoreRuntimeCloseAllDoesNotMarkUnconfirmedPartialFailuresClosed(t *testing.T) {
	t.Parallel()

	backing := partialRuntimeCloseStore{Store: NewMemStore()}
	first, err := backing.Create(Bead{Title: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := backing.Create(Bead{Title: "second"})
	if err != nil {
		t.Fatal(err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.PrimeActive(); err != nil {
		t.Fatalf("PrimeActive: %v", err)
	}

	n, err := RuntimeCloseAll(context.Background(), cache, []string{first.ID, second.ID}, map[string]string{"close_reason": "partial close test"},
		RuntimeWritePolicy(WriteClassHotState, "test.cache-runtime-close-partial", "partial-close"))
	if n != 1 {
		t.Fatalf("RuntimeCloseAll closed = %d, want 1", n)
	}
	if err == nil {
		t.Fatal("RuntimeCloseAll err = nil, want partial failure")
	}

	gotFirst, err := RuntimeGet(context.Background(), cache, first.ID, RuntimeReadPolicy(ReadClassHotAuthoritative, "test.partial-first"))
	if err != nil {
		t.Fatalf("first RuntimeGet: %v", err)
	}
	if gotFirst.Status != "closed" {
		t.Fatalf("first status = %q, want closed after confirmed close", gotFirst.Status)
	}
	if _, err := RuntimeGet(context.Background(), cache, second.ID, RuntimeReadPolicy(ReadClassHotAuthoritative, "test.partial-second")); !IsDegradedRead(err) {
		t.Fatalf("second RuntimeGet err = %v, want degraded dirty cache instead of hidden close", err)
	}
}

func TestCachingStoreRuntimeCloseAllUncachedConfirmedCloseRemainsVisible(t *testing.T) {
	t.Parallel()

	backing := NewMemStore()
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.PrimeActive(); err != nil {
		t.Fatalf("PrimeActive: %v", err)
	}
	bead, err := backing.Create(Bead{Title: "created outside cache"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	n, err := RuntimeCloseAll(context.Background(), cache, []string{bead.ID}, map[string]string{"close_reason": "uncached close test"},
		RuntimeWritePolicy(WriteClassHotState, "test.cache-runtime-close-uncached", "uncached-close"))
	if err != nil {
		t.Fatalf("RuntimeCloseAll: %v", err)
	}
	if n != 1 {
		t.Fatalf("RuntimeCloseAll closed = %d, want 1", n)
	}

	if _, err := RuntimeGet(context.Background(), cache, bead.ID,
		RuntimeReadPolicy(ReadClassHotAuthoritative, "test.uncached-close-runtime-get")); !IsDegradedRead(err) {
		t.Fatalf("RuntimeGet err = %v, want degraded dirty cache instead of hidden tombstone", err)
	}
	got, err := cache.Get(bead.ID)
	if err != nil {
		t.Fatalf("Get closed bead after runtime close: %v", err)
	}
	if got.Status != "closed" {
		t.Fatalf("status = %q, want closed", got.Status)
	}
	gotRuntime, err := RuntimeGet(context.Background(), cache, bead.ID,
		RuntimeReadPolicy(ReadClassHotAuthoritative, "test.uncached-close-runtime-get-after-hydrate"))
	if err != nil {
		t.Fatalf("RuntimeGet after hydrate: %v", err)
	}
	if gotRuntime.Status != "closed" {
		t.Fatalf("runtime status after hydrate = %q, want closed", gotRuntime.Status)
	}
}

func TestCachingStoreRuntimeUpdateDoesNotShortCircuitDirtyCachedMatch(t *testing.T) {
	t.Parallel()

	backing := &runtimeWriteCountingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{
		Title:    "dirty no-op update",
		Status:   "in_progress",
		Metadata: map[string]string{"state": "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.PrimeActive(); err != nil {
		t.Fatalf("PrimeActive: %v", err)
	}
	markCacheDirtyForTest(cache, bead.ID)

	if err := RuntimeUpdate(context.Background(), cache, bead.ID, UpdateOpts{
		Metadata: map[string]string{"state": "active"},
	}, RuntimeWritePolicy(WriteClassHotState, "test.dirty-update", "dirty-update")); err != nil {
		t.Fatalf("RuntimeUpdate: %v", err)
	}
	if backing.runtimeUpdateCalls != 1 {
		t.Fatalf("runtime update calls = %d, want dirty cached match to write through", backing.runtimeUpdateCalls)
	}
}

func TestCachingStoreRuntimeCloseAllDoesNotShortCircuitDirtyCachedClosed(t *testing.T) {
	t.Parallel()

	backing := &runtimeWriteCountingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{
		Title:  "dirty no-op close",
		Status: "closed",
	})
	if err != nil {
		t.Fatal(err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	markCacheDirtyForTest(cache, bead.ID)

	if _, err := RuntimeCloseAll(context.Background(), cache, []string{bead.ID}, nil,
		RuntimeWritePolicy(WriteClassHotState, "test.dirty-close", "dirty-close")); err != nil {
		t.Fatalf("RuntimeCloseAll: %v", err)
	}
	if backing.runtimeCloseAllCalls != 1 {
		t.Fatalf("runtime close-all calls = %d, want dirty cached close to write through", backing.runtimeCloseAllCalls)
	}
}

func TestCachingStoreSetMetadataDoesNotShortCircuitDirtyCachedMatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{
		Title:    "dirty metadata",
		Metadata: map[string]string{"state": "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	markCacheDirtyForTest(cache, bead.ID)
	backing.setMetadataCalls = 0

	if err := cache.SetMetadata(bead.ID, "state", "active"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}
	if backing.setMetadataCalls != 1 {
		t.Fatalf("SetMetadata calls = %d, want dirty cached match to write through", backing.setMetadataCalls)
	}
}

func markCacheDirtyForTest(cache *CachingStore, id string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.dirty[id] = struct{}{}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// TestCachingStoreSetMetadataSkipsBackingWhenCachedValueMatches verifies that
// SetMetadata short-circuits before the backing call when the cached bead
// already has metadata[key]==value. Without this guard, no-op writes still
// fire bd's on_update hook and emit a bead.updated event.
func TestCachingStoreSetMetadataSkipsBackingWhenCachedValueMatches(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := backing.SetMetadata(bead.ID, "foo", "bar"); err != nil {
		t.Fatalf("seed SetMetadata: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.setMetadataCalls = 0

	if err := cache.SetMetadata(bead.ID, "foo", "bar"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}
	if backing.setMetadataCalls != 0 {
		t.Errorf("backing.SetMetadata called %d times; want 0 (no-op write must short-circuit)",
			backing.setMetadataCalls)
	}
}

// TestCachingStoreSetMetadataFallsThroughOnValueMismatch verifies that a
// real value change still propagates to the backing store.
func TestCachingStoreSetMetadataFallsThroughOnValueMismatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := backing.SetMetadata(bead.ID, "foo", "old"); err != nil {
		t.Fatalf("seed SetMetadata: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.setMetadataCalls = 0

	if err := cache.SetMetadata(bead.ID, "foo", "new"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}
	if backing.setMetadataCalls != 1 {
		t.Errorf("backing.SetMetadata called %d times; want 1 (real change must propagate)",
			backing.setMetadataCalls)
	}
}

// TestCachingStoreSetMetadataFallsThroughOnCacheMiss verifies that
// SetMetadata calls the backing store when the cache has no entry for the
// bead — without a primed copy we cannot prove the write is a no-op.
func TestCachingStoreSetMetadataFallsThroughOnCacheMiss(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	bead, err := backing.Create(Bead{Title: "post-prime"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	backing.setMetadataCalls = 0

	if err := cache.SetMetadata(bead.ID, "foo", "bar"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}
	if backing.setMetadataCalls != 1 {
		t.Errorf("backing.SetMetadata called %d times; want 1 (cache miss must fall through)",
			backing.setMetadataCalls)
	}
}

// TestCachingStoreSetMetadataBatchSkipsBackingWhenAllCachedValuesMatch
// verifies that SetMetadataBatch short-circuits when every kv pair already
// matches the cached metadata.
func TestCachingStoreSetMetadataBatchSkipsBackingWhenAllCachedValuesMatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for k, v := range map[string]string{"foo": "1", "bar": "2", "baz": "3"} {
		if err := backing.SetMetadata(bead.ID, k, v); err != nil {
			t.Fatalf("seed SetMetadata(%s): %v", k, err)
		}
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.setMetadataBatchCalls = 0

	if err := cache.SetMetadataBatch(bead.ID, map[string]string{"foo": "1", "bar": "2"}); err != nil {
		t.Fatalf("SetMetadataBatch: %v", err)
	}
	if backing.setMetadataBatchCalls != 0 {
		t.Errorf("backing.SetMetadataBatch called %d times; want 0 (all-match must short-circuit)",
			backing.setMetadataBatchCalls)
	}
}

// TestCachingStoreSetMetadataBatchFallsThroughOnAnyMismatch verifies that
// even one mismatching kv forces the backing call — partial matches do not
// suffice to skip the write.
func TestCachingStoreSetMetadataBatchFallsThroughOnAnyMismatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for k, v := range map[string]string{"foo": "1", "bar": "2"} {
		if err := backing.SetMetadata(bead.ID, k, v); err != nil {
			t.Fatalf("seed SetMetadata(%s): %v", k, err)
		}
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.setMetadataBatchCalls = 0

	// foo matches the cached value, bar does not. The mismatch must force
	// the full batch to the backing store.
	if err := cache.SetMetadataBatch(bead.ID, map[string]string{"foo": "1", "bar": "DIFFERENT"}); err != nil {
		t.Fatalf("SetMetadataBatch: %v", err)
	}
	if backing.setMetadataBatchCalls != 1 {
		t.Errorf("backing.SetMetadataBatch called %d times; want 1 (mismatch must propagate)",
			backing.setMetadataBatchCalls)
	}
}

// TestCachingStoreSetMetadataBatchEmptyKVsIsNoop verifies that an empty kvs
// map returns nil immediately without calling the backing store. This is
// the early-return branch before metadataAlreadyMatchesCached.
func TestCachingStoreSetMetadataBatchEmptyKVsIsNoop(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.setMetadataBatchCalls = 0

	if err := cache.SetMetadataBatch(bead.ID, map[string]string{}); err != nil {
		t.Fatalf("SetMetadataBatch(empty): %v", err)
	}
	if backing.setMetadataBatchCalls != 0 {
		t.Errorf("backing.SetMetadataBatch called %d times; want 0 (empty kvs must short-circuit)",
			backing.setMetadataBatchCalls)
	}
}

// TestCachingStoreUpdateSkipsBackingWhenAllFieldsMatch verifies that Update
// short-circuits before the backing call when every non-nil opts field
// already matches the cached bead. Without this guard the reconciler's
// per-tick Update calls fire bd subprocesses + post-Get refreshes even when
// the payload is identical. See gastownhall/gascity#1978 Phase 1.
func TestCachingStoreUpdateSkipsBackingWhenAllFieldsMatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test", Assignee: "alice"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.updateCalls = 0

	assignee := "alice"
	if err := cache.Update(bead.ID, UpdateOpts{Assignee: &assignee}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if backing.updateCalls != 0 {
		t.Errorf("backing.Update called %d times; want 0 (no-op update must short-circuit)",
			backing.updateCalls)
	}
}

// TestCachingStoreUpdateFallsThroughOnValueMismatch verifies that a real
// field change still propagates to the backing store.
func TestCachingStoreUpdateFallsThroughOnValueMismatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test", Assignee: "alice"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.updateCalls = 0

	assignee := "bob"
	if err := cache.Update(bead.ID, UpdateOpts{Assignee: &assignee}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if backing.updateCalls != 1 {
		t.Errorf("backing.Update called %d times; want 1 (real change must propagate)",
			backing.updateCalls)
	}
}

// TestCachingStoreUpdateFallsThroughOnCacheMiss verifies that Update calls
// the backing store when the cache has no entry for the bead — without a
// primed copy we cannot prove the write is a no-op.
func TestCachingStoreUpdateFallsThroughOnCacheMiss(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	bead, err := backing.Create(Bead{Title: "post-prime", Assignee: "alice"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	backing.updateCalls = 0

	assignee := "alice"
	if err := cache.Update(bead.ID, UpdateOpts{Assignee: &assignee}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if backing.updateCalls != 1 {
		t.Errorf("backing.Update called %d times; want 1 (cache miss must fall through)",
			backing.updateCalls)
	}
}

// TestCachingStoreUpdateFallsThroughOnLabelMismatch verifies that a Labels
// opt requesting a label not yet on the bead still propagates to the backing
// store.
func TestCachingStoreUpdateFallsThroughOnLabelMismatch(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test", Labels: []string{"existing"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.updateCalls = 0

	if err := cache.Update(bead.ID, UpdateOpts{Labels: []string{"new-label"}}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if backing.updateCalls != 1 {
		t.Errorf("backing.Update called %d times; want 1 (new label must propagate)",
			backing.updateCalls)
	}
}

// TestCachingStoreCloseSkipsBackingWhenAlreadyClosed verifies that Close
// short-circuits before the backing call when the cached bead is already
// closed. The cache only holds active beads after Prime, so the close has
// to happen through CachingStore first to seed the closed status into the
// cache. See gastownhall/gascity#1978 Phase 1.
func TestCachingStoreCloseSkipsBackingWhenAlreadyClosed(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	// First close: open → closed, must propagate.
	if err := cache.Close(bead.ID); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if backing.closeCalls != 1 {
		t.Fatalf("backing.Close after first close = %d, want 1", backing.closeCalls)
	}
	backing.closeCalls = 0

	// Second close on the already-closed bead must short-circuit. The
	// reconciler / cleanup paths sometimes re-close the same bead on
	// retry; that should not generate fresh bd subprocess traffic.
	if err := cache.Close(bead.ID); err != nil {
		t.Fatalf("repeat Close: %v", err)
	}
	if backing.closeCalls != 0 {
		t.Errorf("backing.Close called %d times on repeat close; want 0 (already-closed must short-circuit)",
			backing.closeCalls)
	}
}

// TestCachingStoreCloseFallsThroughWhenOpen verifies that a real close still
// propagates to the backing store.
func TestCachingStoreCloseFallsThroughWhenOpen(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	bead, err := backing.Create(Bead{Title: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	backing.closeCalls = 0

	if err := cache.Close(bead.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if backing.closeCalls != 1 {
		t.Errorf("backing.Close called %d times; want 1 (open->closed must propagate)",
			backing.closeCalls)
	}
}

// TestCachingStoreCloseFallsThroughOnCacheMiss verifies that Close calls the
// backing store when the cache has no entry for the bead.
func TestCachingStoreCloseFallsThroughOnCacheMiss(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	bead, err := backing.Create(Bead{Title: "post-prime"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	backing.closeCalls = 0

	if err := cache.Close(bead.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if backing.closeCalls != 1 {
		t.Errorf("backing.Close called %d times; want 1 (cache miss must fall through)",
			backing.closeCalls)
	}
}

// TestCachingStoreCloseAllSkipsBackingWhenAllAlreadyClosed verifies that the
// batch close path has the same no-op protection as Close. Order dispatch uses
// CloseAll for tracking beads; retry/sweep paths can see a cached closed bead
// and must not launch a bd subprocess that can reach Dolt as a no-op commit.
func TestCachingStoreCloseAllSkipsBackingWhenAllAlreadyClosed(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	first, err := backing.Create(Bead{Title: "first"})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	second, err := backing.Create(Bead{Title: "second"})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}

	if closed, err := cache.CloseAll([]string{first.ID, second.ID}, map[string]string{"phase": "done"}); err != nil || closed != 2 {
		t.Fatalf("first CloseAll closed=%d err=%v, want 2 nil", closed, err)
	}
	if backing.closeAllCalls != 1 {
		t.Fatalf("backing.CloseAll after first close = %d, want 1", backing.closeAllCalls)
	}
	backing.closeAllCalls = 0
	backing.closeAllIDs = nil

	closed, err := cache.CloseAll([]string{first.ID, second.ID}, map[string]string{"phase": "done"})
	if err != nil {
		t.Fatalf("repeat CloseAll: %v", err)
	}
	if closed != 0 {
		t.Fatalf("repeat CloseAll closed = %d, want 0 already-closed mutations", closed)
	}
	if backing.closeAllCalls != 0 {
		t.Errorf("backing.CloseAll called %d times on all-closed batch; want 0", backing.closeAllCalls)
	}
	if len(backing.closeAllIDs) != 0 {
		t.Errorf("backing.CloseAll ids = %v, want none", backing.closeAllIDs)
	}
}

// TestCachingStoreCloseAllFiltersAlreadyClosedCachedIDs verifies that a mixed
// batch still closes open IDs while stripping cached closed IDs before the
// backing store sees the batch.
func TestCachingStoreCloseAllFiltersAlreadyClosedCachedIDs(t *testing.T) {
	t.Parallel()

	backing := &countingBackingStore{Store: NewMemStore()}
	closedFirst, err := backing.Create(Bead{Title: "closed first"})
	if err != nil {
		t.Fatalf("Create closedFirst: %v", err)
	}
	openSecond, err := backing.Create(Bead{Title: "open second"})
	if err != nil {
		t.Fatalf("Create openSecond: %v", err)
	}

	cache := NewCachingStoreForTest(backing, nil)
	if err := cache.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	if err := cache.Close(closedFirst.ID); err != nil {
		t.Fatalf("seed Close: %v", err)
	}
	backing.closeAllCalls = 0
	backing.closeAllIDs = nil

	closed, err := cache.CloseAll([]string{closedFirst.ID, openSecond.ID}, map[string]string{"phase": "done"})
	if err != nil {
		t.Fatalf("CloseAll mixed: %v", err)
	}
	if closed != 1 {
		t.Fatalf("CloseAll mixed closed = %d, want 1", closed)
	}
	if backing.closeAllCalls != 1 {
		t.Fatalf("backing.CloseAll calls = %d, want 1", backing.closeAllCalls)
	}
	if len(backing.closeAllIDs) != 1 || backing.closeAllIDs[0] != openSecond.ID {
		t.Fatalf("backing.CloseAll ids = %v, want [%s]", backing.closeAllIDs, openSecond.ID)
	}

	got, err := cache.Get(openSecond.ID)
	if err != nil {
		t.Fatalf("Get openSecond: %v", err)
	}
	if got.Status != "closed" || got.Metadata["phase"] != "done" {
		t.Fatalf("openSecond after CloseAll = status %q metadata %v, want closed phase=done", got.Status, got.Metadata)
	}
}

// TestCachingStoreUpdateSkipsBackingPerFieldMatch is the per-field
// short-circuit coverage requested in gastownhall/gascity#2199. The original
// PR #2159 exercised Assignee + Labels-mismatch + cache-miss only; the
// remaining 6 field branches in updateMatchesCached were asserted by
// inspection. This table-driven test pins the short-circuit behavior for
// each field independently so a future refactor of any single check
// surfaces in CI.
func TestCachingStoreUpdateSkipsBackingPerFieldMatch(t *testing.T) {
	t.Parallel()

	type fieldCase struct {
		name string
		seed Bead
		opts UpdateOpts
	}
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	cases := []fieldCase{
		{
			name: "Title",
			seed: Bead{Title: "pinned"},
			opts: UpdateOpts{Title: strPtr("pinned")},
		},
		{
			name: "Status",
			seed: Bead{Title: "x", Status: "open"},
			opts: UpdateOpts{Status: strPtr("open")},
		},
		{
			name: "Type",
			seed: Bead{Title: "x", Type: "task"},
			opts: UpdateOpts{Type: strPtr("task")},
		},
		{
			name: "Priority",
			seed: Bead{Title: "x", Priority: intPtr(2)},
			opts: UpdateOpts{Priority: intPtr(2)},
		},
		{
			name: "Description",
			seed: Bead{Title: "x", Description: "body"},
			opts: UpdateOpts{Description: strPtr("body")},
		},
		{
			name: "ParentID",
			seed: Bead{Title: "x", ParentID: "gc-parent"},
			opts: UpdateOpts{ParentID: strPtr("gc-parent")},
		},
		{
			name: "Metadata",
			seed: Bead{Title: "x", Metadata: map[string]string{"k": "v"}},
			opts: UpdateOpts{Metadata: map[string]string{"k": "v"}},
		},
		{
			name: "Labels-present",
			seed: Bead{Title: "x", Labels: []string{"a", "b"}},
			opts: UpdateOpts{Labels: []string{"a"}},
		},
		{
			name: "RemoveLabels-absent",
			seed: Bead{Title: "x", Labels: []string{"a"}},
			opts: UpdateOpts{RemoveLabels: []string{"z"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backing := &countingBackingStore{Store: NewMemStore()}
			bead, err := backing.Create(tc.seed)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			cache := NewCachingStoreForTest(backing, nil)
			if err := cache.Prime(context.Background()); err != nil {
				t.Fatalf("Prime: %v", err)
			}
			backing.updateCalls = 0

			if err := cache.Update(bead.ID, tc.opts); err != nil {
				t.Fatalf("Update: %v", err)
			}
			if backing.updateCalls != 0 {
				t.Errorf("backing.Update called %d times; want 0 (%s value-match must short-circuit)",
					backing.updateCalls, tc.name)
			}
		})
	}
}

// TestCachingStoreUpdateFallsThroughPerFieldMismatch is the mismatch-side
// companion to TestCachingStoreUpdateSkipsBackingPerFieldMatch. Each
// subtest asserts that a real change in the named field forces the
// backing call — guarding the matcher against accidentally returning true
// when a single field actually differs.
func TestCachingStoreUpdateFallsThroughPerFieldMismatch(t *testing.T) {
	t.Parallel()

	type fieldCase struct {
		name string
		seed Bead
		opts UpdateOpts
	}
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	cases := []fieldCase{
		{
			name: "Title",
			seed: Bead{Title: "before"},
			opts: UpdateOpts{Title: strPtr("after")},
		},
		{
			name: "Status",
			seed: Bead{Title: "x", Status: "open"},
			opts: UpdateOpts{Status: strPtr("closed")},
		},
		{
			name: "Type",
			seed: Bead{Title: "x", Type: "task"},
			opts: UpdateOpts{Type: strPtr("epic")},
		},
		{
			name: "Priority",
			seed: Bead{Title: "x", Priority: intPtr(2)},
			opts: UpdateOpts{Priority: intPtr(3)},
		},
		{
			name: "Priority-nil-cached",
			seed: Bead{Title: "x"},
			opts: UpdateOpts{Priority: intPtr(2)},
		},
		{
			name: "Description",
			seed: Bead{Title: "x", Description: "before"},
			opts: UpdateOpts{Description: strPtr("after")},
		},
		{
			name: "ParentID",
			seed: Bead{Title: "x", ParentID: "gc-a"},
			opts: UpdateOpts{ParentID: strPtr("gc-b")},
		},
		{
			name: "Metadata-value",
			seed: Bead{Title: "x", Metadata: map[string]string{"k": "old"}},
			opts: UpdateOpts{Metadata: map[string]string{"k": "new"}},
		},
		{
			name: "Metadata-missing-key",
			seed: Bead{Title: "x"},
			opts: UpdateOpts{Metadata: map[string]string{"k": "v"}},
		},
		{
			name: "RemoveLabels-present",
			seed: Bead{Title: "x", Labels: []string{"a", "b"}},
			opts: UpdateOpts{RemoveLabels: []string{"a"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backing := &countingBackingStore{Store: NewMemStore()}
			bead, err := backing.Create(tc.seed)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			cache := NewCachingStoreForTest(backing, nil)
			if err := cache.Prime(context.Background()); err != nil {
				t.Fatalf("Prime: %v", err)
			}
			backing.updateCalls = 0

			if err := cache.Update(bead.ID, tc.opts); err != nil {
				t.Fatalf("Update: %v", err)
			}
			if backing.updateCalls != 1 {
				t.Errorf("backing.Update called %d times; want 1 (%s real change must propagate)",
					backing.updateCalls, tc.name)
			}
		})
	}
}
