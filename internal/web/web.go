// Package web embeds the built admin panel (server/admin → internal/web/dist), so one binary serves it all.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Panel is the admin panel's files (index.html at the root).
func Panel() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
