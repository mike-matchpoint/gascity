// Package domainhandoff embeds the deterministic domain-to-city handoff
// lifecycle primitives (work dispatch, command waiters, terminal publication).
package domainhandoff

import "embed"

// PackFS contains the domain-handoff pack files.
//
//go:embed pack.toml orders template-fragments all:schemas all:assets
var PackFS embed.FS
