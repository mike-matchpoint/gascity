// Package teloscodegen embeds the telos-codegen pack: the telos layer's
// priming/conscience lane for code-generating roles.
package teloscodegen

import "embed"

// PackFS contains the telos-codegen pack files.
//
//go:embed pack.toml README.md template-fragments
var PackFS embed.FS
