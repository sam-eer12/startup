// Package fixtures embeds synthetic provider inputs only. Golden oracle outputs
// are kept separate and are never served by the provider simulator.
package fixtures

import "embed"

//go:embed synthetic/*/*.json
var Files embed.FS
