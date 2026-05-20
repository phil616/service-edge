// Package web embeds the built frontend (web/dist copied into ./dist) so the
// control plane can serve the SPA from a single binary. Before `make web` has
// run, dist contains only a placeholder page.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// FS returns the embedded frontend rooted at the dist directory.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
