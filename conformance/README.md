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

## Running

```
go test ./conformance/
```

`m0_demo_test.go` additionally builds the real `cljgo` binary and pipes
the M0 exit demo (design/00 §6) through `cljgo repl`, asserting the
factorial result and live re-def end to end. `m1_demo_test.go` does the
same for the M1 exit demo: a `defmacro` typed at the prompt working on
the next form, plus `defn`/`when`/`->` from the embedded core.clj.
