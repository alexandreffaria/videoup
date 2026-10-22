//go:build noembed || !(windows || linux || darwin)

package main

import "embed"

// A build without the upscaler inside (go build -tags noembed): it uses an installed
// Upscayl or --upscaler instead.
var assets *embed.FS
