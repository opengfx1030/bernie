package gpu

import "embed"

// devicesFS holds the device tables, one file per GPU generation
// (devices/<generation>.yaml).
//
//go:embed devices/*.yaml
var devicesFS embed.FS
