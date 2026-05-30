// Copyright (c) Gas City contributors. SPDX-License-Identifier: Apache-2.0

package main

import "github.com/gastownhall/gascity/internal/routedwork"

const (
	poolDemandMetadataKey   = routedwork.PoolDemandMetadataKey
	poolDemandMetadataValue = routedwork.PoolDemandOrderValue
)

func poolDemandMetadataPair() map[string]string {
	return routedwork.PoolDemandMetadataPair()
}
