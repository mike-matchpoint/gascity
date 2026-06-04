// Package executioncityoperations embeds generic execution-city operations primitives.
package executioncityoperations

import "embed"

// PackFS contains the execution-city-operations pack files.
//
//go:embed pack.toml all:agents template-fragments all:schemas all:assets
var PackFS embed.FS
