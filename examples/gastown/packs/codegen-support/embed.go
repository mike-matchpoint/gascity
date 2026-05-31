// Package codegensupport embeds the built-in code-generation support pack.
package codegensupport

import "embed"

// PackFS is the embedded codegen-support pack.
//
//go:embed pack.toml formulas orders all:agents all:assets template-fragments
var PackFS embed.FS
