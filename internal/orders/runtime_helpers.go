package orders

import (
	"context"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
)

// LastRunFuncForStore returns the latest order-run bead time for one store.
func LastRunFuncForStore(store beads.Store) LastRunFunc {
	return func(name string) (time.Time, error) {
		if store == nil {
			return time.Time{}, nil
		}
		label := "order-run:" + name
		results, err := store.List(beads.ListQuery{
			Label:         label,
			Limit:         1,
			IncludeClosed: true,
			Sort:          beads.SortCreatedDesc,
		})
		if err != nil {
			return time.Time{}, err
		}
		if len(results) == 0 {
			return time.Time{}, nil
		}
		return results[0].CreatedAt, nil
	}
}

// LastRunAcrossStores returns the most recent run time across a set of stores
// for a single order name.
func LastRunAcrossStores(stores ...beads.Store) LastRunFunc {
	return func(name string) (time.Time, error) {
		var latest time.Time
		for _, store := range stores {
			if store == nil {
				continue
			}
			last, err := LastRunFuncForStore(store)(name)
			if err != nil {
				return time.Time{}, err
			}
			if last.After(latest) {
				latest = last
			}
		}
		return latest, nil
	}
}

// RuntimeLastRunFuncForStore returns the latest order-run bead time through
// the bounded runtime read contract. It is intended for hot health surfaces
// that need durable history visibility without falling back to long bd reads.
func RuntimeLastRunFuncForStore(store beads.Store, timeout time.Duration, caller string) LastRunFunc {
	return func(name string) (time.Time, error) {
		if store == nil {
			return time.Time{}, nil
		}
		if timeout <= 0 {
			timeout = beads.HotAuthoritativeBudget
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		policy := beads.RuntimeReadPolicy(beads.ReadClassHotAuthoritative, caller)
		policy.Timeout = timeout
		policy.MaxRows = 2
		results, err := beads.RuntimeList(ctx, store, beads.ListQuery{
			Label:         "order-run:" + name,
			Limit:         1,
			IncludeClosed: true,
			Sort:          beads.SortCreatedDesc,
			TierMode:      beads.TierBoth,
		}, policy)
		if err != nil {
			return time.Time{}, err
		}
		if len(results) == 0 {
			return time.Time{}, nil
		}
		return results[0].CreatedAt, nil
	}
}

// RuntimeLastRunAcrossStores returns the most recent bounded runtime-read
// order-run time across stores for a single order name.
func RuntimeLastRunAcrossStores(timeout time.Duration, caller string, stores ...beads.Store) LastRunFunc {
	return func(name string) (time.Time, error) {
		var latest time.Time
		for _, store := range stores {
			if store == nil {
				continue
			}
			last, err := RuntimeLastRunFuncForStore(store, timeout, caller)(name)
			if err != nil {
				return time.Time{}, err
			}
			if last.After(latest) {
				latest = last
			}
		}
		return latest, nil
	}
}

// CursorFuncForStore returns the max order-run seq for one store.
func CursorFuncForStore(store beads.Store) CursorFunc {
	return func(name string) uint64 {
		if store == nil {
			return 0
		}
		label := "order-run:" + name
		results, err := store.List(beads.ListQuery{
			Label:         label,
			Limit:         10,
			IncludeClosed: true,
			Sort:          beads.SortCreatedDesc,
		})
		if err != nil || len(results) == 0 {
			return 0
		}
		labelSets := make([][]string, 0, len(results))
		for _, b := range results {
			labelSets = append(labelSets, b.Labels)
		}
		return MaxSeqFromLabels(labelSets)
	}
}

// CursorAcrossStores merges seq cursors from multiple stores.
func CursorAcrossStores(stores ...beads.Store) CursorFunc {
	fns := make([]CursorFunc, 0, len(stores))
	for _, store := range stores {
		if store != nil {
			fns = append(fns, CursorFuncForStore(store))
		}
	}
	return func(name string) uint64 {
		var latest uint64
		for _, fn := range fns {
			if seq := fn(name); seq > latest {
				latest = seq
			}
		}
		return latest
	}
}
