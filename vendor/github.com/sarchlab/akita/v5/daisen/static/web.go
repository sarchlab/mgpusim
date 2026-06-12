// Package static includes the static web pages for Daisen.
package static

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist/*
var staticAssets embed.FS

// GetAssets returns the static assets
func GetAssets() http.FileSystem {
	subFS, err := fs.Sub(staticAssets, "dist")
	if err != nil {
		panic(err)
	}

	return http.FS(subFS)
}
