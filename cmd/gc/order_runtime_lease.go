package main

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/orders"
)

const (
	orderRuntimeLeaseDir            = ".gc/runtime/order-dispatch/leases"
	orderRuntimeLeaseVersion        = "v2"
	orderRuntimeReservationTTL      = 2 * time.Minute
	orderRuntimeLeaseHeartbeatEvery = 10 * time.Second
)

type orderRuntimeLeaseState string

const (
	orderRuntimeLeaseReserving                orderRuntimeLeaseState = "reserving"
	orderRuntimeLeaseActive                   orderRuntimeLeaseState = "active"
	orderRuntimeLeaseCompletedPendingCritical orderRuntimeLeaseState = "completed_pending_critical"
	orderRuntimeLeaseCompletedPendingAudit    orderRuntimeLeaseState = "completed_pending_audit"
	orderRuntimeLeaseAbandoned                orderRuntimeLeaseState = "abandoned"
	orderRuntimeLeaseExpired                  orderRuntimeLeaseState = "expired"
)

type orderRuntimeLease struct {
	Order                     string                 `json:"order"`
	ScopedOrder               string                 `json:"scoped_order"`
	StoreKey                  string                 `json:"store_key"`
	LeaseID                   string                 `json:"lease_id"`
	FencingToken              string                 `json:"fencing_token"`
	Attempt                   int                    `json:"attempt"`
	TriggerKind               string                 `json:"trigger_kind"`
	TriggerFingerprint        string                 `json:"trigger_fingerprint"`
	TrackingRequired          bool                   `json:"tracking_required"`
	TrackingDegradedAllowed   bool                   `json:"tracking_degraded_allowed"`
	TrackingBeadID            string                 `json:"tracking_bead_id"`
	ReservationHash           string                 `json:"reservation_hash"`
	State                     orderRuntimeLeaseState `json:"state"`
	CreatedAt                 time.Time              `json:"created_at"`
	ExpiresAt                 time.Time              `json:"expires_at"`
	StartedAt                 time.Time              `json:"started_at"`
	CompletedAt               time.Time              `json:"completed_at"`
	LastHeartbeat             time.Time              `json:"last_heartbeat"`
	ControllerPID             int                    `json:"controller_pid"`
	ControllerStartID         string                 `json:"controller_start_id"`
	ActionPID                 int                    `json:"action_pid"`
	EventSeq                  uint64                 `json:"event_seq"`
	PostActionCriticalPending string                 `json:"post_action_critical_pending"`
	AuditPending              string                 `json:"audit_pending"`
	LastError                 string                 `json:"last_error"`
}

type orderRuntimeReservation struct {
	Lease       orderRuntimeLease
	Input       string
	Hash        string
	TrackingID  string
	Fingerprint string
}

type orderRuntimeLeaseStore struct {
	dir      string
	lockPath string
	now      func() time.Time
	mu       sync.Mutex
	memory   map[string]orderRuntimeLease
}

func newOrderRuntimeLeaseStore(cityPath string) *orderRuntimeLeaseStore {
	dir := ""
	lockPath := ""
	if strings.TrimSpace(cityPath) != "" {
		dir = filepath.Join(cityPath, orderRuntimeLeaseDir)
		lockPath = filepath.Join(filepath.Dir(dir), "leases.lock")
	}
	return &orderRuntimeLeaseStore{
		dir:      dir,
		lockPath: lockPath,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (s *orderRuntimeLeaseStore) acquire(lease orderRuntimeLease) (orderRuntimeLease, bool, error) {
	if s == nil {
		return lease, true, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockDiskLocked()
	if err != nil {
		return orderRuntimeLease{}, false, err
	}
	defer unlock()

	now := s.nowUTC()
	if err := s.sweepExpiredLocked(now); err != nil {
		return orderRuntimeLease{}, false, err
	}
	leases, err := s.listLocked()
	if err != nil {
		return orderRuntimeLease{}, false, err
	}
	for _, existing := range leases {
		if existing.suppresses(lease.ScopedOrder, lease.TriggerFingerprint, now) {
			return existing, false, nil
		}
	}

	if lease.State == "" {
		lease.State = orderRuntimeLeaseReserving
	}
	if lease.CreatedAt.IsZero() {
		lease.CreatedAt = now
	}
	if lease.ExpiresAt.IsZero() {
		lease.ExpiresAt = now.Add(orderRuntimeReservationTTL)
	}
	if lease.LastHeartbeat.IsZero() {
		lease.LastHeartbeat = now
	}
	if lease.LeaseID == "" {
		return orderRuntimeLease{}, false, errors.New("order runtime lease requires lease_id")
	}
	if s.dir == "" {
		if s.memory == nil {
			s.memory = make(map[string]orderRuntimeLease)
		}
		if existing, ok := s.memory[lease.LeaseID]; ok && existing.suppresses(lease.ScopedOrder, lease.TriggerFingerprint, now) {
			return existing, false, nil
		}
		s.memory[lease.LeaseID] = lease
		return lease, true, nil
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return orderRuntimeLease{}, false, err
	}
	path := s.pathFor(lease.LeaseID)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		existing, readErr := s.readFile(path)
		if readErr != nil {
			return orderRuntimeLease{}, false, readErr
		}
		if existing.suppresses(lease.ScopedOrder, lease.TriggerFingerprint, now) {
			return existing, false, nil
		}
		if err := s.writeReplaceLocked(path, lease); err != nil {
			return orderRuntimeLease{}, false, err
		}
		return lease, true, nil
	}
	if err != nil {
		return orderRuntimeLease{}, false, err
	}
	encErr := json.NewEncoder(f).Encode(lease)
	closeErr := f.Close()
	if encErr != nil {
		_ = os.Remove(path)
		return orderRuntimeLease{}, false, encErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return orderRuntimeLease{}, false, closeErr
	}
	return lease, true, nil
}

func (s *orderRuntimeLeaseStore) markActive(leaseID string, timeout time.Duration) {
	_ = s.update(leaseID, func(lease *orderRuntimeLease, now time.Time) {
		lease.State = orderRuntimeLeaseActive
		lease.StartedAt = now
		lease.LastHeartbeat = now
		if timeout <= 0 {
			timeout = orderRuntimeReservationTTL
		}
		lease.ExpiresAt = now.Add(timeout + orderRuntimeReservationTTL)
		lease.LastError = ""
	})
}

func (s *orderRuntimeLeaseStore) heartbeat(leaseID string) {
	_ = s.update(leaseID, func(lease *orderRuntimeLease, now time.Time) {
		if lease.State == orderRuntimeLeaseActive {
			lease.LastHeartbeat = now
		}
	})
}

func (s *orderRuntimeLeaseStore) markCompletedPendingAudit(leaseID, pending string, err error) {
	_ = s.update(leaseID, func(lease *orderRuntimeLease, now time.Time) {
		lease.State = orderRuntimeLeaseCompletedPendingAudit
		lease.CompletedAt = now
		lease.AuditPending = pending
		lease.LastError = errorString(err)
		lease.ExpiresAt = now.Add(24 * time.Hour)
	})
}

func (s *orderRuntimeLeaseStore) markCompletedPendingCritical(leaseID, pending string, err error) {
	_ = s.update(leaseID, func(lease *orderRuntimeLease, now time.Time) {
		lease.State = orderRuntimeLeaseCompletedPendingCritical
		lease.CompletedAt = now
		lease.PostActionCriticalPending = pending
		lease.LastError = errorString(err)
		lease.ExpiresAt = now.Add(30 * time.Minute)
	})
}

func (s *orderRuntimeLeaseStore) markAbandoned(leaseID string, err error) {
	_ = s.update(leaseID, func(lease *orderRuntimeLease, now time.Time) {
		lease.State = orderRuntimeLeaseAbandoned
		lease.CompletedAt = now
		lease.LastError = errorString(err)
		lease.ExpiresAt = now
	})
}

func (s *orderRuntimeLeaseStore) markReservationDegraded(leaseID string, err error) {
	_ = s.update(leaseID, func(lease *orderRuntimeLease, now time.Time) {
		lease.State = orderRuntimeLeaseReserving
		lease.LastError = errorString(err)
		if lease.CreatedAt.IsZero() {
			lease.CreatedAt = now
		}
		lease.ExpiresAt = lease.CreatedAt.Add(orderRuntimeReservationTTL)
	})
}

func (s *orderRuntimeLeaseStore) completeAndRemove(leaseID string) {
	if s == nil || leaseID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockDiskLocked()
	if err != nil {
		return
	}
	defer unlock()
	if s.dir == "" {
		delete(s.memory, leaseID)
		return
	}
	_ = os.Remove(s.pathFor(leaseID))
}

func (s *orderRuntimeLeaseStore) load(leaseID string) (orderRuntimeLease, bool) {
	if s == nil || leaseID == "" {
		return orderRuntimeLease{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockDiskLocked()
	if err != nil {
		return orderRuntimeLease{}, false
	}
	defer unlock()
	if s.dir == "" {
		lease, ok := s.memory[leaseID]
		return lease, ok
	}
	lease, err := s.readFile(s.pathFor(leaseID))
	return lease, err == nil
}

func (s *orderRuntimeLeaseStore) count() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockDiskLocked()
	if err != nil {
		return 0
	}
	defer unlock()
	leases, err := s.listLocked()
	if err != nil {
		return 0
	}
	return len(leases)
}

func (s *orderRuntimeLeaseStore) update(leaseID string, mutate func(*orderRuntimeLease, time.Time)) error {
	if s == nil || leaseID == "" || mutate == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockDiskLocked()
	if err != nil {
		return err
	}
	defer unlock()
	now := s.nowUTC()
	var lease orderRuntimeLease
	if s.dir == "" {
		if s.memory == nil {
			s.memory = make(map[string]orderRuntimeLease)
		}
		lease = s.memory[leaseID]
		if lease.LeaseID == "" {
			lease.LeaseID = leaseID
		}
		mutate(&lease, now)
		s.memory[leaseID] = lease
		return nil
	}
	path := s.pathFor(leaseID)
	lease, err = s.readFile(path)
	if err != nil {
		return err
	}
	mutate(&lease, now)
	return s.writeReplaceLocked(path, lease)
}

func (s *orderRuntimeLeaseStore) lockDiskLocked() (func(), error) {
	if s == nil || s.dir == "" || s.lockPath == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.lockPath), 0o700); err != nil {
		return nil, err
	}
	locker := beads.NewFileFlock(s.lockPath)
	if err := locker.Lock(); err != nil {
		return nil, err
	}
	return func() { _ = locker.Unlock() }, nil
}

func (s *orderRuntimeLeaseStore) sweepExpiredLocked(now time.Time) error {
	leases, err := s.listLocked()
	if err != nil {
		return err
	}
	for _, lease := range leases {
		if lease.LeaseID == "" || lease.ExpiresAt.IsZero() || now.Before(lease.ExpiresAt) {
			continue
		}
		if lease.State == orderRuntimeLeaseAbandoned || lease.State == orderRuntimeLeaseExpired {
			continue
		}
		lease.State = orderRuntimeLeaseExpired
		lease.LastError = "runtime lease expired"
		if s.dir == "" {
			s.memory[lease.LeaseID] = lease
			continue
		}
		if err := s.writeReplaceLocked(s.pathFor(lease.LeaseID), lease); err != nil {
			return err
		}
	}
	return nil
}

func (s *orderRuntimeLeaseStore) listLocked() ([]orderRuntimeLease, error) {
	if s == nil {
		return nil, nil
	}
	if s.dir == "" {
		out := make([]orderRuntimeLease, 0, len(s.memory))
		for _, lease := range s.memory {
			out = append(out, lease)
		}
		return out, nil
	}
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []orderRuntimeLease
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		lease, err := s.readFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}
		out = append(out, lease)
	}
	return out, nil
}

func (s *orderRuntimeLeaseStore) writeReplaceLocked(path string, lease orderRuntimeLease) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	encErr := json.NewEncoder(tmp).Encode(lease)
	closeErr := tmp.Close()
	if encErr != nil {
		_ = os.Remove(tmpName)
		return encErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpName)
		return closeErr
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func (s *orderRuntimeLeaseStore) readFile(path string) (orderRuntimeLease, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return orderRuntimeLease{}, err
	}
	var lease orderRuntimeLease
	if err := json.Unmarshal(data, &lease); err != nil {
		return orderRuntimeLease{}, err
	}
	return lease, nil
}

func (s *orderRuntimeLeaseStore) pathFor(leaseID string) string {
	return filepath.Join(s.dir, safeOrderRuntimeLeaseFile(leaseID)+".json")
}

func (s *orderRuntimeLeaseStore) nowUTC() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func (l orderRuntimeLease) suppresses(scopedOrder, triggerFingerprint string, now time.Time) bool {
	if l.ScopedOrder != scopedOrder {
		return false
	}
	if !l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt) {
		return false
	}
	switch l.State {
	case orderRuntimeLeaseReserving:
		return l.TriggerFingerprint == triggerFingerprint
	case orderRuntimeLeaseActive:
		return true
	case orderRuntimeLeaseCompletedPendingCritical:
		return l.TriggerFingerprint == triggerFingerprint
	default:
		return false
	}
}

func safeOrderRuntimeLeaseFile(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		sum := sha256.Sum256([]byte(value))
		return hex.EncodeToString(sum[:8])
	}
	return b.String()
}

func orderRuntimeReservationFor(a orders.Order, storeKey, storePrefix string, now time.Time, eventSeq uint64) orderRuntimeReservation {
	const attempt = 1
	fingerprint := orderTriggerFingerprint(a, now, eventSeq)
	trackingPrefix := orderRuntimeTrackingIDPrefix(storePrefix)
	input := strings.Join([]string{
		orderRuntimeLeaseVersion,
		strings.TrimSpace(storeKey),
		trackingPrefix,
		a.ScopedName(),
		a.Trigger,
		fingerprint,
		fmt.Sprintf("%d", attempt),
	}, "\x00")
	sum := sha256.Sum256([]byte(input))
	hash := hex.EncodeToString(sum[:])
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:16])
	trackingID := trackingPrefix + "-order-" + strings.ToLower(encoded)
	return orderRuntimeReservation{
		Input:       input,
		Hash:        hash,
		TrackingID:  trackingID,
		Fingerprint: fingerprint,
		Lease: orderRuntimeLease{
			Order:                   a.Name,
			ScopedOrder:             a.ScopedName(),
			StoreKey:                storeKey,
			LeaseID:                 trackingID,
			FencingToken:            hash,
			Attempt:                 attempt,
			TriggerKind:             a.Trigger,
			TriggerFingerprint:      fingerprint,
			TrackingRequired:        !a.TrackingDegradedAllowed,
			TrackingDegradedAllowed: a.TrackingDegradedAllowed,
			TrackingBeadID:          trackingID,
			ReservationHash:         hash,
			State:                   orderRuntimeLeaseReserving,
			ControllerPID:           os.Getpid(),
			EventSeq:                eventSeq,
		},
	}
}

func orderRuntimeTrackingIDPrefix(prefix string) string {
	prefix = strings.Trim(strings.ToLower(strings.TrimSpace(prefix)), "-")
	if prefix == "" {
		return "gc"
	}
	return prefix
}

func orderTriggerFingerprint(a orders.Order, now time.Time, eventSeq uint64) string {
	now = now.UTC()
	switch a.Trigger {
	case "cooldown":
		interval, err := time.ParseDuration(a.Interval)
		if err != nil || interval <= 0 {
			return "cooldown:" + a.Interval + ":invalid"
		}
		return fmt.Sprintf("cooldown:%s:%d", interval, now.UnixNano()/interval.Nanoseconds())
	case "cron":
		return "cron:" + a.Schedule + ":" + now.Truncate(time.Minute).Format(time.RFC3339)
	case "event":
		return fmt.Sprintf("event:%s:%d", a.On, eventSeq)
	case "condition":
		sum := sha256.Sum256([]byte(a.Check))
		return fmt.Sprintf("condition:%x:%d", sum[:], now.Unix()/60)
	default:
		return a.Trigger + ":" + now.Format(time.RFC3339Nano)
	}
}

func orderDegradedMinInterval(a orders.Order) time.Duration {
	if strings.TrimSpace(a.DegradedMinInterval) == "" {
		return 0
	}
	d, err := time.ParseDuration(a.DegradedMinInterval)
	if err != nil {
		return 0
	}
	return d
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
