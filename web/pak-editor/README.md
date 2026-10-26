# pak-editor UI

React + TypeScript + Vite frontend for `cmd/pak-editor`. The production build is
written to `../../cmd/pak-editor/ui`, which the Go binary embeds.

```sh
npm install
npm run dev:mock   # UI only, in-memory fake backend (VITE_MOCK=1 via .env.mock)
npm run dev        # real backend: first run `go run ./cmd/pak-editor --port 5174 --no-browser`
                   # then open http://localhost:5173/?t=<token printed by pak-editor>
npm run build      # typecheck + build into cmd/pak-editor/ui (then rebuild the Go binary)
npm test           # vitest
npm run typecheck
```

The dev server proxies `/api` to `http://127.0.0.1:5174`.
