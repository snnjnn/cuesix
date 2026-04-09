This frontend is a static UI for the sixpack schema/control API.

The API contract is generated from annotations and exposed via OpenAPI at `/schema/openapi/doc.json` (source in `cmd/sixpack/control/api.go`).

Tech stack:
- Alpine.js for application state and interactions.
- Bulma for layout/styling plus DataTables Bulma integration for index browsing.
- CodeMirror 6 for YAML viewing/editing and keyboard-driven validation.
- esbuild for bundling static assets.

Runtime model:
- Static assets only (no server-side runtime in this package).
- UI is served under `/schema/app/*` by the sixpack process.
- Keep UI practical and operations-focused for DevOps users.

Modes:
1. Browse: list and open source snippets from `GET /schema/sources` and `GET /schema/sources/{path}` in a read-only YAML editor.
2. Index: browse config objects by APISIX kind/id from `GET /schema/virtualgw/{virtualgw}`, then open merged YAML for an object via `GET /schema/virtualgw/{virtualgw}/{kind}/{id}.3. Playground: edit YAML freely in the editor.

Validation behavior:
- Browse mode validates by source path using `GET /schema/validate/{path}`.
- Playground validates inline payload using `POST /schema/validate`.
- Query parameters are treated as environment overrides in both modes and must be shared across mode switches.

Implementation notes:
- Endpoint base paths are defined in `src/constants.js` and are intentionally relative (`../validate`, `../sources`, `../virtualgw/default`) because the app is mounted under `/schema/app/`.
- Keep HTML semantic and CSS overrides minimal; prefer built-in Bulma components.
