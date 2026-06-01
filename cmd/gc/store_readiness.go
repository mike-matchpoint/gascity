package main

import (
	"context"
	"time"

	"github.com/gastownhall/gascity/internal/beads"
)

func checkStoreForRuntimeReadiness(store beads.Store, deadline time.Time, caller string) error {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		remaining = time.Nanosecond
	}
	timeout := beads.HotDegradedOKBudget
	if remaining < timeout {
		timeout = remaining
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	policy := beads.RuntimeReadPolicy(beads.ReadClassHotDegradedOK, caller)
	policy.Timeout = timeout
	policy.MaxRows = 1
	if counter, ok := store.(interface {
		CountIndexed(context.Context, beads.ListQuery) (int, error)
	}); ok {
		_, err := counter.CountIndexed(ctx, beads.ListQuery{Status: "open"})
		return err
	}
	_, err := beads.RuntimeList(ctx, store, beads.ListQuery{
		Status: "open",
		Limit:  1,
	}, policy)
	return err
}
