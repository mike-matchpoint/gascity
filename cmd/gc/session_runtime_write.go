package main

import (
	"context"
	"errors"

	"github.com/gastownhall/gascity/internal/beads"
)

func sessionRuntimeReadPolicy(caller string) beads.ReadPolicy {
	return beads.RuntimeReadPolicy(beads.ReadClassHotAuthoritative, caller)
}

func sessionRuntimeWritePolicy(id, caller string) beads.WritePolicy {
	return beads.RuntimeWritePolicy(beads.WriteClassHotState, caller, "session:"+id)
}

func runtimeGetSessionBead(ctx context.Context, store beads.Store, id, caller string) (beads.Bead, error) {
	b, err := beads.RuntimeGet(ctx, store, id, sessionRuntimeReadPolicy(caller))
	if err != nil && errors.Is(err, beads.ErrIndexedListUnsupported) && store != nil {
		return store.Get(id)
	}
	return b, err
}

func runtimeUpdateSessionBead(ctx context.Context, store beads.Store, id string, opts beads.UpdateOpts, caller string) error {
	return beads.RuntimeUpdate(ctx, store, id, opts, sessionRuntimeWritePolicy(id, caller))
}

func runtimeSetSessionMetadata(ctx context.Context, store beads.Store, id, key, value, caller string) error {
	err := runtimeUpdateSessionBead(ctx, store, id, beads.UpdateOpts{Metadata: map[string]string{key: value}}, caller)
	if err != nil && errors.Is(err, beads.ErrRuntimeWriteUnsupported) && store != nil {
		return store.SetMetadata(id, key, value)
	}
	return err
}

func runtimeSetSessionMetadataBatch(ctx context.Context, store beads.Store, id string, batch map[string]string, caller string) error {
	if len(batch) == 0 {
		return nil
	}
	err := runtimeUpdateSessionBead(ctx, store, id, beads.UpdateOpts{Metadata: batch}, caller)
	if err != nil && errors.Is(err, beads.ErrRuntimeWriteUnsupported) && store != nil {
		return store.SetMetadataBatch(id, batch)
	}
	return err
}

func runtimeCloseSessionBeads(ctx context.Context, store beads.Store, ids []string, metadata map[string]string, caller string) (int, error) {
	idempotencyKey := "session:close"
	if len(ids) == 1 {
		idempotencyKey = "session:" + ids[0] + ":close"
	}
	policy := beads.RuntimeWritePolicy(beads.WriteClassHotState, caller, idempotencyKey)
	n, err := beads.RuntimeCloseAll(ctx, store, ids, metadata, policy)
	if err != nil && errors.Is(err, beads.ErrRuntimeWriteUnsupported) && store != nil {
		return store.CloseAll(ids, metadata)
	}
	return n, err
}
