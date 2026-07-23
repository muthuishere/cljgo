# Tasks — apply-adr-0071-bri-aot-docker

Spike s45 (spikes/s45-bri-aot-docker) validates 1–4 before the template/docs
land. The order matters: bri must COMPILE (task 1) before parity (3), Docker
(4), and benchmark (5) mean anything.

## 1. bri emits + links in the compiled path (the enablement)

- [x] 1.1 Make `core/bri/*.cljg` resolve + emit at `cljgo build` the way
  `core/async.cljg` does (they are already embedded via `core/bri.go`). A user
  `(require '[bri.http])` must not error at compile. — DONE. `cmd/genbri`
  AOT-compiles the bri sources into `pkg/briaot/<ns>` (mirrors `cmd/gencore`);
  `CompileProgram` registers the bri lib providers on the discovery evaluator so
  `(require '[bri.http])` resolves at build (`pkg/emit/module.go`).
- [x] 1.2 Register the `pkg/bri` host shims (`-serve`/`-request`/`-json-*`/
  `-jwt-*`/`-argon2-*`/`-hmac-sign`/`-rand-token`/`-getenv`/…) in the COMPILED
  path via an emitted-package lib provider (ADR 0042 §2); the generated go.mod
  requires the bri runtime pkg when the app uses bri. Verify no shim reaches
  back into `pkg/eval` (sever any interpreter dependency the spike surfaces). —
  DONE. `pkg/bri` is now pure Go (the eval-coupled loader split to
  `pkg/briloader`); `pkg/briaot` registers a provider per bri ns that installs
  the shims then runs the compiled forms. `pkg/bri` is the SAME module as the
  runtime (replace), so the blank import suffices; `SynthGoMod` now carries the
  runtime's external requires (`golang.org/x/crypto`) + copies its `go.sum` so a
  dev build links bri offline. `TestNoInterpreterInCompiledBinary` extended to
  `pkg/briaot` + `pkg/bri` — proves no `pkg/eval` edge.
- [x] 1.3 Emitted `func main()` invokes the app `-main`. `cljgo build` on a bri
  hello-world → a static binary that serves `GET /` (text) + `GET /api/hello`
  (JSON). `CGO_ENABLED=0`; `ldd`/`otool -L` scratch-clean. Gates green. — DONE.
  The existing `func main()` `-main` dispatch works once bri links; a bri app's
  `-main` calls `http/listen` and serves. linux/amd64+arm64 builds are
  `ELF … statically linked … stripped`; no `_cgo_*` symbols. See `bri/NOTES.md`.

## 2. Spike proof (s45)

- [x] 2.1 `spikes/s45-bri-aot-docker/bri/`: the bri hello app (src + build) that
  compiles to a static binary and serves; record binary size + startup. — DONE.
  App in `bri/src/app/`; `build.cljgo`; binary ~10.6–11.5 MB static, ~16 ms
  startup-to-first-200. Recorded in `bri/NOTES.md`.

## 3. Dual-mode parity harness

- [x] 3.1 A bri behavior suite runs the SAME app interpreted (`cljgo dev` / the
  in-process client) AND compiled, asserting byte-identical responses for: a
  text route, a JSON route, a JWT-guarded route (401→200), and a funnel case
  (bad param → 400). A divergence fails. Gates green. — DONE. `bri/parity/run.sh`
  drives `app.parity` interpreted (`cljgo run`) AND compiled (`cljgo build`) over
  all four cases (+ good param 200) and diffs; PARITY OK, byte-identical.

## 4. Docker

- [ ] 4.1 `templates/web`: multi-stage `Dockerfile` (Go build → scratch/
  distroless) + `.dockerignore`; the generated web app builds an image that
  serves `/` + `/healthz`. Update the web template manifest + templates_test.go.
  `spikes/s45-bri-aot-docker/bri/Dockerfile` mirrors it for the spike. Gates
  green.

## 5. Benchmark + VERDICT (spike s45, serial, one machine)

- [ ] 5.1 Comparison corpus (`compare/`): Go net/http, Ring+Jetty, http-kit,
  Bun, Node, Deno — equivalent hello+JSON servers, each Dockerized. (Corpus
  agent.)
- [ ] 5.2 Serial benchmark runner (`bench/`): warmed load test (oha/hey, fixed
  duration + concurrency) capturing req/s, p50/p99, peak RSS, image size,
  cold-start for every runtime on `/` and `/api/hello`. Never run two at once.
- [ ] 5.3 `spikes/s45-bri-aot-docker/VERDICT.md`: the table + the one-line call
  (is compiled bri.http clearly ahead of JVM Clojure web?). Feeds ADR 0071
  accept/stop.

## 6. Rollout (only if the VERDICT ratifies ADR 0071)

- [ ] 6.1 README: a bri Docker deploy section + the perf table. Flip ADR 0071
  to accepted. Update the template `build.cljgo` note (AOT of bri apps now
  ships). Gates green.
