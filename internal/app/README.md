## Playground compilation with esbuild

```bash
npx esbuild app/src/index.js --bundle --outdir=assets/dist --splitting --format=esm --loader:.svg=file
```

Or:

```bash
npm install --save-dev esbuild
go generate ./...
```
