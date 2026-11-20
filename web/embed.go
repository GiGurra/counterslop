// Package web embeds the browser client served by the Go binary.
package web

import "embed"

//go:embed index.html style.css engine ui game gen assets dev vendor
var FS embed.FS
