package devices

import "embed"

// devicesFS holds the device tables, one file per GPU generation
// (<generation>.yaml).
//
//go:embed *.yaml
var devicesFS embed.FS
