// Copyright (c) Gas City contributors. SPDX-License-Identifier: Apache-2.0

package main

import "github.com/gastownhall/gascity/internal/routedwork"

const (
	poolDemandMetadataKey   = routedwork.PoolDemandMetadataKey
	poolDemandMetadataValue = routedwork.PoolDemandOrderValue
)

// poolDemandMetadataPair returns the metadata map a pool-order writer
// must merge into its UpdateOpts.Metadata alongside the existing
// gc.routed_to write. Writers compose with the routing key:
//
//	if a.Pool != "" {
//	    update.Metadata = map[string]string{"gc.routed_to": pool}
//	    for k, v := range poolDemandMetadataPair() {
//	        update.Metadata[k] = v
//	    }
//	}
//
// The helper exists so adding a second flag in the future (e.g., a
// per-trigger discriminator) does not require auditing every writer.
func poolDemandMetadataPair() map[string]string {
	return routedwork.PoolDemandMetadataPair()
}
