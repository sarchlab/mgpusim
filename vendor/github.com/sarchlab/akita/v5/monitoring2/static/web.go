package static

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var assets embed.FS

func GetAssets() http.FileSystem {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}

	return http.FS(sub)
}
