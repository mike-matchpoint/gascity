// Package teloscore embeds the telos-core pack: shared telos-layer primitives
// (SYSTEM-TELOS snapshot pin consumption and common evidence fragments).
package teloscore

import "embed"

// PackFS contains the telos-core pack files.
//
//go:embed pack.toml README.md template-fragments
var PackFS embed.FS
