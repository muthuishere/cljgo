# Spike S15 — minimal nREPL for editor adoption

ADR 0027 pipeline, stage 1. Closes toward **ADR 0031**.

## Question

What is the MINIMAL nREPL op set that gives a working Calva and CIDER
connect/eval/doc loop against cljgo's existing evaluator (`pkg/eval` +
the session machinery in `pkg/repl`), and does anything in our
architecture block it?

Sub-questions this spike must answer for the ADR:

1. **Op list for v1** — what must `describe` advertise for Calva to be
   fully functional and CIDER to degrade gracefully (no cider-nrepl
   middleware)?
2. **Bencode dependency decision** — nREPL's wire format is bencode.
   cljgo has zero external Go deps (only golang.org/x/tools for the
   emitter). Vendor a codec, or write our own (~150 lines)?
3. **Where the server lives** — new `pkg/nrepl` fronting `pkg/repl` /
   `pkg/eval`? What seams do the existing packages already give us
   (sessions, `*1 *2 *3 *e`, Interrupt), and what's missing?
4. **Interrupt** — `pkg/repl.Driver.Interrupt` discards pending INPUT;
   can a running eval be aborted at all with today's tree-walk
   evaluator, and if not, what would it need?
5. **Architectural blockers** — anything in the runtime (goroutine-local
   dynamic bindings, global namespace world, evaluator thread-safety)
   that makes nREPL sessions impossible or expensive?

## Exit criterion (written before any code, per ADR 0027)

A **scripted nREPL client session** — raw bencode over a TCP socket,
driving `clone` → `describe` → `eval "(+ 1 2)"` → `load-file` →
`interrupt` — **round-trips against the prototype server** (every
request gets the spec-shaped response, eval returns `"3"`, load-file
defines vars that a later eval sees, interrupt answers honestly), PLUS
a **written op-gap list for Calva and for CIDER** (what each client
sends on connect, which ops we cover, which we stub, which we lack).
Verdict and recommendation land in `VERDICT.md`.

## Prior art studied

- **babashka.nrepl** (the proof that a small op set is enough for real
  editor adoption). Its server dispatches exactly: `clone`, `close`,
  `eval`, `load-file`, `complete`/`completions`, `lookup`/`info`,
  `eldoc`, `describe`, `ns-list`, `ls-sessions` (verified against
  `babashka/babashka.nrepl` `src/babashka/nrepl/impl/server.clj`,
  2026-07-16). Notably it ships **no `interrupt`** and Calva works.
- **CIDER** additionally evals `clojure.main/repl-requires` on connect
  (documented babashka gotcha) and probes `describe` for cider-nrepl
  middleware ops; it warns and degrades when they're absent.

## Layout

- `bencode.go` — self-written bencode codec (the dependency decision,
  measured in lines).
- `server.go` — prototype TCP nREPL server fronting `pkg/eval`, with
  sessions as goroutines owning `lang.PushThreadBindings` frames
  (the same `*ns*` / `*1 *2 *3 *e` model `pkg/repl.Driver.Run` uses).
- `main.go` — `go run . -port 1667` for real-editor smoke tests.
- `bencode_test.go` / `session_test.go` — codec round-trip + the
  scripted client session above (the exit criterion, runnable as
  `go test ./...` inside this dir; own go.mod keeps it out of the
  root module's gates).

## How to run

```
cd spikes/s15-nrepl-minimal
go test ./...          # codec + scripted session (exit criterion)
go run . -port 1667    # then connect Calva / CIDER to localhost:1667
```
