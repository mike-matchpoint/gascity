// Package telosexecmonitoring embeds the telos-exec-monitoring pack: the
// telos layer's effectiveness/TELOS-GAP telemetry emitters (findings only,
// never verdicts).
package telosexecmonitoring

import "embed"

// PackFS contains the telos-exec-monitoring pack files.
//
//go:embed pack.toml README.md template-fragments
var PackFS embed.FS
