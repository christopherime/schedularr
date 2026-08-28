// Package web embeds the Hugo-built site (web/public) into the schedularr
// binary. Run `make web` to regenerate web/public from the Hugo sources in
// web/layouts, web/assets, and web/content before building for release; a
// committed placeholder keeps `go build ./...` working without Hugo.
package web

import (
	"embed"
	"io/fs"
)

// FS is the Hugo build output (web/public), embedded at compile time.
// Prefer Site() over using FS directly -- it strips the "public" prefix.
//
//go:embed all:public
var FS embed.FS

// Site returns the embedded site rooted at web/public, suitable for
// mounting directly on an HTTP file server.
func Site() (fs.FS, error) {
	return fs.Sub(FS, "public")
}
