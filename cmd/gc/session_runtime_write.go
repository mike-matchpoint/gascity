package main

import (
	"context"

	"github.com/gastownhall/gascity/internal/beads"
)

func sessionRuntimeReadPolicy(caller string) beads.ReadPolicy {
	return beads.RuntimeReadPolicy(beads.ReadClassHotAuthoritative, caller)
}

func sessionRuntimeWritePolicy(id, caller string) beads.WritePolicy {
	return beads.RuntimeWritePolicy(beads.WriteClassHotState, caller, "session:"+id)
}

func runtimeGetSessionBead(ctx context.Context, store beads.Store, id, caller string) (beads.Bead, error) {
	return beads.RuntimeGet(ctx, store, id, sessionRuntimeReadPolicy(caller))
}

func runtimeUpdateSessionBead(ctx context.Context, store beads.Store, id string, opts beads.UpdateOpts, caller string) error {
	return beads.RuntimeUpdate(ctx, store, id, opts, sessionRuntimeWritePolicy(id, caller))
}

func runtimeSetSessionMetadata(ctx context.Context, store beads.Store, id, key, value, caller string) error {
	return runtimeSetSessionMetadataBatch(ctx, store, id, map[string]string{key: value}, caller)
}

func runtimeSetSessionMetadataBatch(ctx context.Context, store beads.Store, id string, batch map[string]string, caller string) error {
	if len(batch) == 0 {
		return nil
	}
	return runtimeUpdateSessionBead(ctx, store, id, beads.UpdateOpts{Metadata: batch}, caller)
}

func runtimeCloseSessionBeads(ctx context.Context, store beads.Store, ids []string, metadata map[string]string, caller string) (int, error) {
	idempotencyKey := "session:close"
	if len(ids) == 1 {
		idempotencyKey = "session:" + ids[0] + ":close"
	}
	policy := beads.RuntimeWritePolicy(beads.WriteClassHotState, caller, idempotencyKey)
	return beads.RuntimeCloseAll(ctx, store, ids, metadata, policy)
}
