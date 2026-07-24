# Conformance suite

The dual-mode conformance suite of design/00 §6 / design/03 §7d, in its
first (M0, eval-only) form. Every semantic test is a plain `.clj` file
under `tests/`; the Go harness (`conformance_test.go`) runs each file
through the same Read → Analyze → Eval path the REPL uses (the
`pkg/repl` driver). At M2 the Go-emitter harness joins and every file
runs through **both** paths — a file that can't run both ways will need
a written waiver in the file.

## File format

A test file is ordinary Clojure source: any number of forms, evaluated
in order in a fresh `user` namespace. Exactly one expectation comment,
conventionally trailing:

- `;; expect: <text>` — the `pr-str` of the **last form's value** must
  equal `<text>` exactly.
- `;; expect-error: <text>` — evaluating the file must fail, and the
  error message must **contain** `<text>` (position prefixes vary, so
  substring match).

Anything else starting with `;;` is an ordinary comment. Keep one
behavior per file; the filename names the behavior.

Multi-namespace tests (ADR 0042): a test file may `require` namespaces
whose sources live in a subdirectory of `tests/` (e.g. `tests/conf/` for
ns `conf.*`) — subdirectories are outside the harness glob, so those
files load only via require, in both harnesses. Oracle such tests with
`clojure -Sdeps '{:paths ["."]}' -M <file>` from `tests/`.

## Harness directives

A `;;`-comment line may carry a per-file directive:

- `;; harness: eval — <reason>` — run ONLY the interpreted harness; skip
  the compiled (AOT-binary) leg. For behaviors with no compiled contract
  yet (e.g. runtime-error output).
- `;; oracle: skip — <reason>` — the `ORACLE=1` re-audit does not compare
  this file against the real `clojure` CLI (documented cljgo deviations).
- `;; harness: standalone — <reason>` — the compiled leg builds this file
  as its **own** binary instead of sharing a batched group binary. The
  compiled harness (`compiled_test.go`) links most single-file programs
  into a handful of shared group binaries and runs each in its own
  process — one shared binary means one macOS first-exec penalty instead
  of hundreds, the dominant cost of the suite. Programs whose output
  depends on the **process-global** namespace/keyword registry (`ns-map`,
  `all-ns`, `ns-publics` counts, membership of common short names, …)
  must opt out: sibling programs in a shared binary intern their vars at
  package-init, polluting that registry. Correctness is never at risk if
  you forget the marker — the eval-vs-binary divergence assertion fails
  loudly (it can never false-pass), pointing you here.

## Running

```
go test ./conformance/
```

`m0_demo_test.go` additionally builds the real `cljgo` binary and pipes
the M0 exit demo (design/00 §6) through `cljgo repl`, asserting the
factorial result and live re-def end to end. `m1_demo_test.go` does the
same for the M1 exit demo: a `defmacro` typed at the prompt working on
the next form, plus `defn`/`when`/`->` from the embedded core.clj.
