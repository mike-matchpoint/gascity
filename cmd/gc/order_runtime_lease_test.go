package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/orders"
)

func TestOrderRuntimeReservationDeterministicID(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	order := orders.Order{Name: "beads-health", Trigger: "cooldown", Interval: "1m"}

	first := orderRuntimeReservationFor(order, "city:file:/tmp/store", "vgc", now, 0)
	second := orderRuntimeReservationFor(order, "city:file:/tmp/store", "vgc", now, 0)
	if first.TrackingID != second.TrackingID {
		t.Fatalf("TrackingID mismatch: %q != %q", first.TrackingID, second.TrackingID)
	}
	if first.Hash != second.Hash || first.Input != second.Input {
		t.Fatalf("reservation hash/input not deterministic")
	}
	if !strings.HasPrefix(first.TrackingID, "vgc-order-") {
		t.Fatalf("TrackingID = %q, want vgc-order- prefix", first.TrackingID)
	}
	if first.TrackingID != strings.ToLower(first.TrackingID) || strings.Contains(first.TrackingID, "=") {
		t.Fatalf("TrackingID = %q, want lowercase unpadded base32", first.TrackingID)
	}

	nextBucket := orderRuntimeReservationFor(order, "city:file:/tmp/store", "vgc", now.Add(time.Minute), 0)
	if first.TrackingID == nextBucket.TrackingID {
		t.Fatalf("TrackingID did not change across cooldown bucket: %q", first.TrackingID)
	}

	noPrefix := orderRuntimeReservationFor(order, "city:file:/tmp/store", "", now, 0)
	if !strings.HasPrefix(noPrefix.TrackingID, "gc-order-") {
		t.Fatalf("TrackingID without store prefix = %q, want gc-order- fallback", noPrefix.TrackingID)
	}
}

func TestOrderRuntimeLeaseStoreSuppressionStates(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		state         orderRuntimeLeaseState
		expiresAt     time.Time
		fingerprint   string
		wantSame      bool
		wantDifferent bool
	}{
		{
			name:          "reserving same fingerprint only",
			state:         orderRuntimeLeaseReserving,
			expiresAt:     now.Add(time.Minute),
			fingerprint:   "fp-a",
			wantSame:      true,
			wantDifferent: false,
		},
		{
			name:          "active any fingerprint",
			state:         orderRuntimeLeaseActive,
			expiresAt:     now.Add(time.Minute),
			fingerprint:   "fp-a",
			wantSame:      true,
			wantDifferent: true,
		},
		{
			name:          "critical same fingerprint only",
			state:         orderRuntimeLeaseCompletedPendingCritical,
			expiresAt:     now.Add(time.Minute),
			fingerprint:   "fp-a",
			wantSame:      true,
			wantDifferent: false,
		},
		{
			name:          "audit pending does not suppress",
			state:         orderRuntimeLeaseCompletedPendingAudit,
			expiresAt:     now.Add(time.Minute),
			fingerprint:   "fp-a",
			wantSame:      false,
			wantDifferent: false,
		},
		{
			name:          "abandoned does not suppress",
			state:         orderRuntimeLeaseAbandoned,
			expiresAt:     now.Add(time.Minute),
			fingerprint:   "fp-a",
			wantSame:      false,
			wantDifferent: false,
		},
		{
			name:          "expired does not suppress",
			state:         orderRuntimeLeaseActive,
			expiresAt:     now.Add(-time.Second),
			fingerprint:   "fp-a",
			wantSame:      false,
			wantDifferent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/same", func(t *testing.T) {
			store := newOrderRuntimeLeaseStore("")
			store.now = func() time.Time { return now }
			mustAcquireOrderRuntimeLease(t, store, orderRuntimeLease{
				Order:              "health",
				ScopedOrder:        "health",
				LeaseID:            "existing",
				TriggerFingerprint: tt.fingerprint,
				State:              tt.state,
				CreatedAt:          now.Add(-time.Second),
				ExpiresAt:          tt.expiresAt,
			})
			_, acquired, err := store.acquire(orderRuntimeLease{
				Order:              "health",
				ScopedOrder:        "health",
				LeaseID:            "candidate",
				TriggerFingerprint: tt.fingerprint,
			})
			if err != nil {
				t.Fatalf("acquire candidate: %v", err)
			}
			if suppressed := !acquired; suppressed != tt.wantSame {
				t.Fatalf("suppressed = %v, want %v", suppressed, tt.wantSame)
			}
		})
		t.Run(tt.name+"/different", func(t *testing.T) {
			store := newOrderRuntimeLeaseStore("")
			store.now = func() time.Time { return now }
			mustAcquireOrderRuntimeLease(t, store, orderRuntimeLease{
				Order:              "health",
				ScopedOrder:        "health",
				LeaseID:            "existing",
				TriggerFingerprint: tt.fingerprint,
				State:              tt.state,
				CreatedAt:          now.Add(-time.Second),
				ExpiresAt:          tt.expiresAt,
			})
			_, acquired, err := store.acquire(orderRuntimeLease{
				Order:              "health",
				ScopedOrder:        "health",
				LeaseID:            "candidate",
				TriggerFingerprint: "fp-b",
			})
			if err != nil {
				t.Fatalf("acquire candidate: %v", err)
			}
			if suppressed := !acquired; suppressed != tt.wantDifferent {
				t.Fatalf("suppressed = %v, want %v", suppressed, tt.wantDifferent)
			}
		})
	}
}

func TestOrderRuntimeLeaseStoreMarksExpiredLease(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	store := newOrderRuntimeLeaseStore("")
	store.now = func() time.Time { return now }
	mustAcquireOrderRuntimeLease(t, store, orderRuntimeLease{
		Order:              "health",
		ScopedOrder:        "health",
		LeaseID:            "existing",
		TriggerFingerprint: "fp-a",
		State:              orderRuntimeLeaseActive,
		CreatedAt:          now.Add(-time.Hour),
		ExpiresAt:          now.Add(-time.Second),
	})

	_, acquired, err := store.acquire(orderRuntimeLease{
		Order:              "health",
		ScopedOrder:        "health",
		LeaseID:            "candidate",
		TriggerFingerprint: "fp-a",
	})
	if err != nil {
		t.Fatalf("acquire candidate: %v", err)
	}
	if !acquired {
		t.Fatal("expired active lease suppressed candidate")
	}
	expired, ok := store.load("existing")
	if !ok {
		t.Fatal("expired lease missing")
	}
	if expired.State != orderRuntimeLeaseExpired {
		t.Fatalf("expired state = %q, want %q", expired.State, orderRuntimeLeaseExpired)
	}
}

func TestOrderRuntimeLeaseStoreFilePersistence(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cityDir := t.TempDir()
	store := newOrderRuntimeLeaseStore(cityDir)
	store.now = func() time.Time { return now }
	lease := mustAcquireOrderRuntimeLease(t, store, orderRuntimeLease{
		Order:              "health",
		ScopedOrder:        "health",
		StoreKey:           "city:file",
		LeaseID:            "gc-order-test",
		TriggerFingerprint: "fp-a",
		State:              orderRuntimeLeaseReserving,
		ExpiresAt:          now.Add(time.Minute),
	})

	reopened := newOrderRuntimeLeaseStore(cityDir)
	got, ok := reopened.load(lease.LeaseID)
	if !ok {
		t.Fatal("persisted lease missing after reopening store")
	}
	if got.ScopedOrder != lease.ScopedOrder || got.StoreKey != lease.StoreKey || got.State != lease.State {
		t.Fatalf("persisted lease = %#v, want %#v", got, lease)
	}
}

func TestOrderRuntimeLeaseStoreFileExpiredSameIDReacquires(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	store := newOrderRuntimeLeaseStore(t.TempDir())
	store.now = func() time.Time { return now }
	mustAcquireOrderRuntimeLease(t, store, orderRuntimeLease{
		Order:              "health",
		ScopedOrder:        "health",
		LeaseID:            "gc-order-same",
		TriggerFingerprint: "fp-a",
		State:              orderRuntimeLeaseActive,
		CreatedAt:          now.Add(-time.Hour),
		ExpiresAt:          now.Add(-time.Second),
	})

	lease, acquired, err := store.acquire(orderRuntimeLease{
		Order:              "health",
		ScopedOrder:        "health",
		LeaseID:            "gc-order-same",
		TriggerFingerprint: "fp-a",
		State:              orderRuntimeLeaseReserving,
		ExpiresAt:          now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("acquire replacement: %v", err)
	}
	if !acquired {
		t.Fatalf("expired same-ID file lease suppressed reacquire: %#v", lease)
	}
	if lease.State != orderRuntimeLeaseReserving {
		t.Fatalf("replacement state = %q, want %q", lease.State, orderRuntimeLeaseReserving)
	}
}

func mustAcquireOrderRuntimeLease(t *testing.T, store *orderRuntimeLeaseStore, lease orderRuntimeLease) orderRuntimeLease {
	t.Helper()
	got, acquired, err := store.acquire(lease)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if !acquired {
		t.Fatalf("lease %q was unexpectedly suppressed", lease.LeaseID)
	}
	return got
}
