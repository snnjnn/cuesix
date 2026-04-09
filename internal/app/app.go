package app

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:generate npx esbuild app/src/index.js --bundle --outdir=assets/dist --splitting --format=esm --loader:.svg=file
//go:embed assets/dist
var playgroundFS embed.FS

// AppHandler serves the embedded playground static assets.
func AppHandler() http.Handler {
	sub, _ := fs.Sub(playgroundFS, "assets/dist")
	return http.FileServer(http.FS(sub))
}
