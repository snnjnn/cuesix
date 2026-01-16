package schema

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:generate npx esbuild app/src/index.js --bundle --outdir=app/dist --splitting --format=esm --loader:.svg=file
//go:embed app/dist
var playgroundFS embed.FS

func AppHandler() http.Handler {
	sub, _ := fs.Sub(playgroundFS, "app/dist")
	return http.FileServer(http.FS(sub))
}
