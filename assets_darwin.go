//go:build darwin && !noembed

package main

import "embed"

//go:embed assets/darwin assets/models
var embeddedFS embed.FS

var assets = &embeddedFS
