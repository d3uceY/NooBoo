# Contributing to NooBoo

Bug reports and pull requests are welcome. This is a small desktop tool, so the bar is short: it builds, the tests pass, nothing gets installed silently.

## Setup

Requires Go 1.26+, Node 24+, and the [Wails v3 CLI](https://v3alpha.wails.io/).

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
```

Then:

```bash
go mod tidy
cd frontend && npm install && cd ..
wails3 generate bindings -clean -ts
```

## Running

```bash
wails3 dev
```

Starts the Go backend, the Vite dev server on port 9245, and the window with hot reload.

Build a release binary:

```bash
wails3 build
```

## Tests

```bash
go test ./services/...
go vet ./models/... ./services/... .
```

Tests run against a temporary folder — they never touch a real Sims 4 install. If you add behaviour that touches the player's files, add a test for it in `services/userflow_test.go`, written as the thing a player would actually do.

## Ground rules

- **Never destroy player data.** Removing a mod moves it to quarantine. Restoring a save parks the current one. Anything irreversible needs a test proving the safe path.
- **Ids are paths.** Resolve them and reject anything that escapes the Mods folder.
- **Go stays testable.** Services read their app-data folder from `ConfigService`, so tests can point it at a temp dir.
- **Bindings are generated.** Never edit `frontend/bindings`; run `wails3 generate bindings` after changing a Go service or model.

## Frontend

- React + TypeScript, Tailwind v4, GSAP for motion.
- All Go calls go through `frontend/src/lib/api.ts`. Screens never import bindings directly.
- Reuse the components in `frontend/src/ui`. Don't hand-roll buttons or inputs — if a control needs a new form, add a variant instead.
- Respect `prefers-reduced-motion`.
- UI copy follows [concise-text](https://github.com/wondelai/skills): name the thing, skip the sentence under the label that already says it.

## Commits

Conventional Commits (`feat:`, `fix:`, `chore:`). One logical change per commit.

## Third-party art

The interface icons and app icon come from TheSimKid's *Sims 4 Modder Icons* pack. If you swap artwork, keep the source and instance code recorded in `frontend/src/ui/icons.tsx` so it stays traceable.
