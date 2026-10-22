//go:build windows && !noembed

package main

import "embed"

//go:embed assets/windows assets/models
var embeddedFS embed.FS

var assets = &embeddedFS
