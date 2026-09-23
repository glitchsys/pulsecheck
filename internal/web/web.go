package web

import "embed"

// Files holds HTML templates and CSS compiled into the binary.
//
//go:embed templates static
var Files embed.FS
