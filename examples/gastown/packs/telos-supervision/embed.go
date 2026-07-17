// Package telossupervision embeds the telos-supervision pack: the supervisor
// lane's overseer telos law as an injection fragment for the city's mayor
// role (a fragment, never an agent).
package telossupervision

import "embed"

// PackFS contains the telos-supervision pack files.
//
//go:embed pack.toml README.md template-fragments
var PackFS embed.FS
