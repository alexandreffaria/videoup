//go:build linux && !noembed

package main

import "embed"

//go:embed assets/linux assets/models
var embeddedFS embed.FS

var assets = &embeddedFS
