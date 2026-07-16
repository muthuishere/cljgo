# Spike S15 verdict — minimal nREPL for editor adoption

Closed 2026-07-16. Recommendation feeds **ADR 0031**.

## Exit criterion: MET

- `session_test.go` (`go test ./...` in this dir) drives a scripted
  raw-bencode client over a real TCP socket: clone → describe → eval
  `(+ 1 2)` → `"3"` → println streams an `out` message → `*1` works →
  eval-error shape → load-file defines `spike.s15/twice`, a later eval
  calls it → interrupt answers `session-idle` → lookup `map` returns
  name/ns/arglists → complete `"ma"` finds `map` → second session has
  isolated `*1`/`*ns*` → ls-sessions → close. All green.
- **Bonus, real client**: the actual nREPL client (`clojure -Sdeps
  '{:deps {nrepl/nrepl {:mvn/version "1.3.1"}}}' -M -m nrepl.cmdline
  --connect --port …`) connected to the prototype, evaluated `(+ 1 2)`
  → `3`, streamed `println` output, and rendered our `describe`
  versions ("Clojure 1.12.5"). `(doc map)` failed only because cljgo
  has no `doc` macro yet — an evaluator gap, not an nREPL gap.

## 1. Op list for v1 (what `describe` should advertise)

Babashka-proven minimal surface (verified against
`babashka/babashka.nrepl` `impl/server.clj`, which powers real
Calva/CIDER use):

```
clone close describe eval load-file
complete completions lookup info eldoc
ls-sessions interrupt ns-list
```

All thirteen are implemented (interrupt as an honest stub) in ~525
lines of `server.go` against unmodified `pkg/eval` + `pkg/lang`.

**Op-gap list — Calva** (generic "Connect to a running REPL"):
- Needs: clone, describe, eval (with `ns` targeting), load-file,
  close, interrupt (optional), complete, info/lookup for hover.
- Gaps: **none** — every op Calva sends is covered. babashka ships
  this exact surface (minus interrupt, for years) and Calva is fully
  functional against it.

**Op-gap list — CIDER**:
- Connect handshake (clone + describe + eval) works. CIDER warns
  "cider-nrepl middleware not found" and degrades to nREPL built-in
  `lookup`/`completions` — both covered.
- CIDER evals `clojure.main/repl-requires` on connect; cljgo has no
  `clojure.main` ns and no `doc`/`source`/`apropos`/`dir` vars, so
  that eval errors (gracefully — same documented gotcha as babashka).
  Fix belongs in core, not nREPL: provide those vars.
- Out of scope (cider-nrepl middleware, degrade is graceful):
  `macroexpand`, `test`, `ns-vars`, `apropos`, `classpath`,
  `stacktrace`, debugger ops. Not needed for connect/eval/doc.

## 2. Bencode dependency decision: WRITE IT (163 lines, zero deps)

`bencode.go` is 163 lines including comments, covers everything nREPL
puts on the wire (byte strings, ints, lists, key-sorted dicts), and
round-trips + unicode-safe under test. Vendoring or requiring a
third-party codec buys nothing and would be cljgo's first external
runtime dep (root go.mod has only golang.org/x/tools, emitter-side).
**Recommendation: own the codec inside pkg/nrepl.**

## 3. Where the server lives: new `pkg/nrepl`, sessions = goroutines

- One shared `eval.Evaluator` per server — the namespace/var world in
  `pkg/lang` is process-global anyway, exactly like a JVM nREPL server.
- **The load-bearing trick**: dynamic bindings are goroutine-keyed
  (`lang.PushThreadBindings` / `internal/goid`), so an nREPL session is
  a goroutine that pushes the same frame `repl.Driver.Run` pushes
  (`*ns*` + `*1 *2 *3 *e`) and executes ALL that session's ops on that
  goroutine (channel of closures). Per-session namespaces and result
  history came for free — the session-isolation test proves it.
- `pkg/repl.Driver` itself was NOT directly reusable: `Run` is wired to
  line-oriented stdin, `evalAndPrint` is unexported, `EvalString`
  deliberately skips `*1`-binding. Recommendation for the spec: extract
  the session frame + `*1 *2 *3 *e` shift into a small shared helper
  (e.g. `repl.Session`) that both the terminal driver and `pkg/nrepl`
  front; the driver comment already anticipates "the future nREPL
  server" as a second thin frontend.

## 4. What interrupt needs

Not implementable today. `Driver.Interrupt` only discards pending
*input*; the tree-walk evaluator (`pkg/eval.Eval`) has no cancellation
checkpoint, and Go goroutines can't be killed. The prototype answers
honestly per spec (`session-idle` when idle; when busy the eval keeps
running) — babashka shipped exactly this way for years and editors
cope. Real interrupt = a per-session cancel flag checked at loop/call
checkpoints in the evaluator (cheap atomic load; must also be designed
into AOT-emitted code or REPL/binary behavior diverges — worth its own
line in ADR 0031, maybe deferred).

## 5. Architectural blockers

**None fatal.** Gaps found, in priority order:

1. **`println` writes to the package-global `eval.Out`, not the `*out*`
   dynamic var** (`pkg/eval/builtins.go:14,126`). The prototype swaps
   the global under a server-wide eval mutex — fine for a spike, wrong
   for concurrent sessions. pkg/nrepl needs print builtins to honor
   `lang.VarOut` (root = os.Stdout, so terminal behavior is unchanged).
2. **No `clojure.main` / `doc` / `source`** — CIDER's connect-time
   `repl-requires` eval errors (gracefully). Core work, tracked
   separately.
3. `sync.Map`-style session registry, sessionless-message handling,
   `nrepl://` port file for editor auto-discovery (`.nrepl-port`) —
   all trivial, none architectural.

## Recommendation for ADR 0031

Build `pkg/nrepl` (~700 lines all-in, zero new deps): own bencode
codec, the 13-op surface above, sessions as binding-frame goroutines
over one shared Evaluator, honest no-op interrupt, `.nrepl-port` file,
`cljgo nrepl [--port N]` subcommand in cmd/cljgo. Precondition worth
sequencing first: make print builtins honor `*out*`. Follow-ups, not
blockers: `doc`/`clojure.main` vars for CIDER polish; evaluator
cancellation checkpoints for real interrupt.
