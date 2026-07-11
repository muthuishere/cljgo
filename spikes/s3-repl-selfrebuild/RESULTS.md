VERDICT: WORKS — add-dep→get→genpkg→rebuild→exec is a 1.7–2.9s cycle warm (5.5s absolute worst case, cold caches + fat dep), far under the 15s budget; stdin survives the exec so the flow demos end-to-end in one pipe; state carry needs a replay journal (Glojure carries nothing because its flow runs before the REPL exists).

# S3 — REPL deps self-rebuild UX

## What was built

`s3repl`: a fake REPL (stdin lines) in this directory's own module.

- `:add-dep <module>` → `go get module@latest` → regenerate `zz_registry.go`
  (minimal genpkg: `go/types` walk via `importer.ForCompiler(fset, "source", nil)`,
  exported non-generic **funcs only**, emitted as `register("pkg/path", "Name", pkg.Name)`
  init calls, `go/format`-gated) → `go build -o s3repl.new .` → `os.Rename` over
  the running binary → `syscall.Exec` into it with `-rebooted` + `S3_T0=<t0 nanos>`
  in env. The rebooted binary prints `rebuilt, N symbols available` and the
  measured full-cycle wall clock.
- `:call <alias>/<Func> [string args]` → alias (last path segment) → registry
  lookup → `reflect.ValueOf(fn).Call(...)`, all return values printed.
- `deps.list` persists the module set so every regen covers all deps added so far.

Demo transcript (single pipe, the `:call`s execute in the *rebuilt* process):

```
$ printf ':add-dep github.com/google/uuid\n:call uuid/NewString\n:quit\n' | ./s3repl
user=> ;; go get github.com/google/uuid@latest: 1.24s
;; registry regen (31 syms, 1 pkgs): 0.39s
;; go build: 0.24s
;; exec .../s3repl
rebuilt, 31 symbols available
;; :add-dep cycle wall-clock: 2.34s
user=> 8c50c93f-9d1e-4bb0-af63-abd7aa24d68a
```

`uuid/Parse` with a string arg works (prints `UUID` + `<nil>`, or the zero UUID
+ `invalid UUID length: 10` on bad input) — the `[v err]` shaping surface is
reachable through this registry with plain reflect. Adding a second dep
(`gorilla/websocket`) regenerates a combined registry; the first dep stays
callable after the second rebuild.

## Cycle timings (darwin/arm64, M-series, go1.26.3)

| scenario | go get | regen | build | full cycle |
|---|---|---|---|---|
| warm everything, first dep (uuid) | — | — | — | **2.80s** |
| warm repeat (re-add uuid, 2 pkgs in registry) | 0.25s | 0.70s | 0.29s | **1.71s** |
| fresh GOMODCACHE (real network download), warm GOCACHE | 1.24s | 0.39s | 0.24s | **2.34s** |
| true cold (fresh GOCACHE + GOMODCACHE), uuid | 1.53s | 0.36s | 0.53s | **2.91s** |
| true cold, gorilla/websocket (pulls net/http tree) | 2.81s | 0.70s | 1.50s | **5.50s** |

Residual ~0.5s per cycle = rename + exec + process boot + registry init.
"True cold" assumes the base REPL binary was already built once (which is what
an install step does), so the binary's own stdlib objects are cached; a wholly
virgin GOCACHE would add one ~10s stdlib compile, once per machine, at install
time — never inside the loop.

**Well under the 15s kill-line.** No mitigations *required*, but see below —
regen is the piece that won't scale linearly.

## Registry sizes

| registry | file size | symbols |
|---|---|---|
| uuid (funcs only) | 2,998 B | 31 |
| uuid + websocket (funcs only) | 4,353 B | 42 |

Funcs-only undercounts a real registry. Full exported surface (glojure-style:
consts, vars, types-as-`reflect.Type` too): uuid = 52 objects, websocket = 41
(methods ride along on the registered types via reflection, they are not
separate entries). Extrapolating, a full per-package registry is ~5–10KB — noise.
Reference: Glojure's **entire pre-generated stdlib** registry is ~640KB per
GOOS/GOARCH (`refs/glojure/pkg/gen/gljimports/`). Registry size is a non-issue;
binary size (11.7MB here, dominated by `go/types`+`go/importer` which the real
cljgo binary carries anyway for its compiler) is the real footprint.

## State carry across exec — design (not implemented)

What `syscall.Exec` preserves for free: pid, cwd, env, **open file descriptors**
— proven above: the piped stdin continued feeding the new binary. (Spike gotcha
that matters for real: read stdin *unbuffered* pre-exec, or buffered lookahead
is lost with the old process's heap. The spike reads byte-at-a-time; a real REPL
should hand off its unconsumed input buffer explicitly, e.g. via a tmpfile named
in env.)

What dies: the entire Go heap — namespaces, vars, atoms, interned keywords,
goroutines, channels, timers, Go objects wrapping sockets/files.

**Proposed mechanism — replay journal, not value snapshot:**

1. The REPL appends every successfully-evaluated top-level form to a session
   journal (`$TMPDIR/cljgo-session-<pid>.clj`), verbatim source text + a flag
   for whether eval was known-pure (`def`/`defn`/`defmacro`/`ns` vs anything else).
2. On `:add-dep`, pass the journal path via env; the rebooted binary replays it
   through the normal reader→analyzer→eval path before printing its banner:
   defs replay by default; side-effecting forms are skipped with a printed
   count (opt-in `:replay-all`). Replay through the ordinary eval path keeps
   the "one analyzer, one semantics" invariant — restored state cannot diverge
   from typed-in state.
3. Value snapshotting (print-dup/edn of var values, atoms as
   `(def a (atom <edn>))`) is a *refinement* for expensive-to-recompute data,
   not the foundation: fn values, lazy seqs mid-realization, and anything
   holding a Go value from the old binary are not serializable anyway.

**What cannot survive, ever** (document, print a notice at reboot):
running goroutines / in-flight `go` blocks, open channels and anything blocked
on them, open connections/files as *Go objects* (the fds survive but nothing
re-wraps them — not worth the heroics), dynamic bindings in flight, watchers/
validators (recreated by journal replay), `reflect.Type`s from the old process.

**What Glojure does: nothing.** Its deps flow (`cmd/glj/main.go` +
`internal/deps/`) runs at *process start*: if `gljdeps.edn` exists it regens
and `syscall.Exec`s `go run ./glj/cmd/glj` **before the REPL ever starts**, so
there is no state to carry and no in-session `add-dep` at all — editing
gljdeps.edn mid-session requires a manual restart. Our design is a strict UX
superset: same mechanism, but triggerable live, made to *feel* live by journal
replay. (JVM Clojure 1.12's `add-lib` is the bar: truly live, no restart. Exec
+ replay is the closest a statically-linked host can get.)

## UX assessment

- Sub-3s warm / sub-6s worst-case feels like "a heavy require", not "a
  rebuild". With the reboot banner + journal replay it reads as one continuous
  session. Verdict: the UX story holds.
- The regen step re-typechecks **every** dep on **every** add (0.36s for one
  pkg, 0.70s for two — the source importer re-checks the whole set). Linear
  growth means a 20-dep project pays seconds per add. Mitigation (do this in
  the real impl): **incremental registry** — one `zz_registry_<munged-pkg>.go`
  per package, generate only the new one; plus a persistent export cache keyed
  by `module@version` (type surfaces are immutable per version). Also: use
  `go list -m` (local, ~0ms) instead of `go get` when the module is already in
  go.mod, and prewarm the build cache by building the base binary at install.
- `go get` mutates the project `go.mod` — correct behavior (it *is* the
  deps.edn analog per doc 05 §1; the real impl writes deps.edn and mirrors into
  the generated module).
- Portability note: `syscall.Exec` is POSIX-only; Windows needs
  spawn-child + exit-parent (stdin handoff gets harder — the journal/tmpfile
  approach covers it).
- Build-over-running-binary: write to `<bin>.new` then `os.Rename` — avoids
  ETXTBSY-class issues and is atomic.

## Files

- `main.go` — REPL loop, `:add-dep` (get→regen→build→rename→exec), `:call`
  (reflect), byte-wise stdin reader (exec-safe), `deps.list` persistence.
- `gen.go` — minimal genpkg (`go/types` scope walk, funcs only, format-gated).
- `zz_registry.go`, `deps.list`, `go.mod`/`go.sum`, `s3repl` — generated/built
  state from the demo runs (uuid + websocket registered, 42 symbols).
