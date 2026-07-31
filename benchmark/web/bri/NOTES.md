# s45 — bri hello app: AOT enablement notes

The bri hello app (`src/app/…`) is the spike's exit-criterion-1/2 proof: a bri
web API that **AOT-compiles to one static `CGO_ENABLED=0` binary** and serves
real HTTP, byte-identical to the interpreted app.

## The app
- `src/app/routes.cljg` — the routes (VALUES) + handlers, shared by both
  entries so interpreted and compiled exercise the SAME code:
  - `GET /` → `text/plain` `"hello\n"`
  - `GET /api/hello` → `application/json` `{"msg":"hello from bri"}`
  - `GET /api/secret` → `bri.auth/logged-in-only` (401 without a token, 200 with)
- `src/app/main.cljg` — the real server (`http/listen`, api-defaults: CORS,
  request-ids, `/healthz`, `/metrics`, SIGTERM drain).
- `src/app/parity.cljg` + `src/run_parity.cljg` — the dual-mode harness.

## What ADR 0071 changed (the enablement)
1. `pkg/bri` is now **pure Go** (no `pkg/eval`): shims + `Specs()` +
   `InstallShimsInto`. The interpreter loader moved to `pkg/briloader`.
2. `cmd/genbri` AOT-compiles `core/bri/*.cljg` → `pkg/briaot/<ns>` (mirrors
   `cmd/gencore`/coreaot). `pkg/briaot` registers a lib provider per bri
   namespace that installs the Go shims then runs the compiled forms — no
   interpreter linked (enforced by the extended `TestNoInterpreterInCompiledBinary`).
3. The emitter blank-imports `pkg/briaot` into a bri app's `main` when the app
   uses bri (`CompileProgram` records it), and `SynthGoMod` now carries the
   runtime's external requires (`golang.org/x/crypto` for argon2/bcrypt) + copies
   its `go.sum`, so a replace-based dev build links bri offline.

## Build + serve (measured on this machine, host = darwin/arm64)

`cljgo build src/app/main.cljg` → a static binary that serves:

```
$ APP_PORT=18899 ./bri-hello
$ curl -s http://127.0.0.1:18899/           -> hello
$ curl -s http://127.0.0.1:18899/api/hello  -> {"msg":"hello from bri"}
$ curl -s -o /dev/null -w '%{http_code}' /api/secret   -> 401
$ curl .../healthz -> 200 ; .../metrics -> 200
```

### Binary size (`cljgo build`, `-trimpath -ldflags=-s -w`, CGO_ENABLED=0)
| target            | bytes       | ~MB   |
|-------------------|-------------|-------|
| darwin/arm64 (host) | 11,388,226 | 10.9  |
| linux/arm64 (scratch) | 11,141,282 | 10.6 |
| linux/amd64 (scratch) | 12,017,826 | 11.5 |

### Static-ness
- `CGO_ENABLED=0`; the linux binaries are `ELF … statically linked … stripped`
  (`file` output) — **scratch/distroless-ready, zero dynamic libs**.
- macOS always links `libSystem`/`CoreFoundation`/`Security` (OS policy, not
  cgo); the deploy target is the Linux static binary above.
- `go tool nm` shows no `_cgo_*` symbols.

### Startup
- **~16 ms** exec → first HTTP 200 (process spawn + rt.Boot core load + bind +
  first request), measured via a poll loop. The rt.Boot core-load floor is
  ~5 ms; the rest is process spawn + listener setup.

## Dual-mode parity (exit criterion 2)
`parity/run.sh` runs `app.parity` interpreted (`cljgo run`) AND compiled
(`cljgo build`) and diffs the canonical response lines:

```
GET /             {:status 200, :content-type "text/plain", :body "hello\n"}
GET /api/hello    {:status 200, :content-type "application/json", :body "{\"msg\":\"hello from bri\"}"}
GET /api/secret   {:status 401, :content-type "application/json", :body "{\"error\":\"unauthorized\"}"}
GET /api/secret+  {:status 200, :content-type "application/json", :body "{\"secret\":\"42\",\"sub\":\"ada\"}"}
GET /api/n/7      {:status 200, :content-type "application/json", :body "{\"id\":7}"}
GET /api/n/abc    {:status 400, :content-type "application/json", :body "{\"error\":\"http/bad-param\"}"}
```

**PARITY OK — byte-identical** (text, JSON, JWT 401→200, typed-param funnel
200/400). A divergence exits non-zero (release blocker, CLAUDE.md).

## Not in scope here (later spike tasks)
- Task 4 Docker image build/measure and task 5 the serial benchmark vs the
  `compare/` corpus. `Dockerfile` in this dir mirrors the intended
  multi-stage build for that agent.
