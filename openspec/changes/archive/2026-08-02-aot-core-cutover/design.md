# Design — aot-core-cutover (AOT-core piece 3)

The decisions and their rationale are ADR 0046 (accepted). This file is
the implementation shape and the findings that fed it — including the
two questions design.md §4 of builtins-to-lang left to this piece.

## 1. Why the compile is the boot, captured

core.clj is not a program; it is the FIRST 137 forms of a namespace that
builds itself. Form 3 defines `defn` and form 4 uses it. So the compiler
cannot analyze core.clj against a booted clojure.core (that would be
compiling it against itself); it must analyze it against exactly what
the interpreter has when it reads that same line — which is
`eval.NewBare()` (Go builtins + the hand-built defmacro) plus everything
core.clj has defined so far. That is what compile-time = eval-time
(ADR 0002) already means; gencore just also keeps the analyzed nodes.

The same reasoning chains across files: numeric.cljg analyzes against
core.clj's result, transducers.cljg against numeric's, and so on. Hence
ONE evaluator for the whole generation run, and ONE ordered table
(`core.BootSources()`) that the interpreter also walks — not two lists
that agree today.

## 2. The rt ↔ coreaot cycle, and how the registry breaks it

Emitted core code calls `rt.Add2` etc., so `coreaot → rt`. If `rt.Boot`
imported coreaot to load it, that is an import cycle. ADR 0042 already
solved the same shape for dependency namespaces (a package registers its
Load from init; the requiring package blank-imports it), so piece 3
reuses it verbatim: `coreaot.init()` → `rt.RegisterCoreLoader(Load)`, and
emitted `main` blank-imports `pkg/coreaot`. The linker keeps what main
imports; rt stays a leaf.

Snapshot ordering: `RegisterAll` → snapshot the pristine `+ - * / < > =`
→ `coreLoader()`. The snapshot must precede core's Load because core's
own compiled code calls the intrinsics. Verified: no boot source re-defs
any of the seven (if one ever did, the guard simply takes the
redefined-value path — correct, slower).

## 3. What compiles vs what stays interpreted

Compiled (all 13 of `core.BootSources()`, eagerly loaded, in boot order):
core.clj, numeric, hierarchies, predicates, transducers, protocols
(→ clojure.core); string, set, edn, test, build, portability, repl
(→ their own namespaces).

Eager, not lazy, deliberately: the interpreter loads all 13 at boot, so
`(all-ns)`, `(find-ns 'clojure.test)` and `ns-publics` answer the same in
both modes. Lazy satellites are the obvious next lever (~4.5 ms of the
remaining 6 ms startup is this Load) and are a separate decision with a
parity cost — ADR 0046 Consequences records it rather than smuggling it
in.

**keel stays interpreted.** `core/keel/*.cljg` are not boot sources: they
load lazily on `(require 'keel.http)` through the provider registry, and
`pkg/keel` interns Go shims through an `*eval.Evaluator`. A compiled
binary never links pkg/keel, so keel costs an AOT binary nothing today —
but an AOT keel app needs pkg/keel to lose its pkg/eval edge, which is
its own piece of work (the shims are pure; the source-loading is not).
Compiling it here would have meant either dragging eval back in (defeats
the change) or rewriting keel's loader mid-cutover.

## 4. The four builtins (was five): decided against the oracle

`require` left the list: it is NOT interpreter-coupled — the registry is
what a binary needs, and only source-file loading needs the interpreter,
so that half became a hook (§3.3). The remaining four and their AOT
answers are ADR 0046 §5; the short version:

| builtin | AOT | why |
|---|---|---|
| eval | throws, bound | needs the analyzer; CLJS answers the same |
| macroexpand(-1) | throws, bound | the analyzer's expander |
| require-go | **no-op** | compile-time directive; already linked |

The oracle (`clojure` 1.12.5, 2026-07-17) says `(eval (list '+ 1 2))` =>
3 even AOT — but a JVM program always links clojure.jar, so the JVM has
no compiler-less artifact and therefore no opinion here. The honest
precedent is the model cljgo actually follows (CLJS): no `eval` in
cljs.core, macroexpansion compile-time only, runtime eval = a separate
opt-in artifact. Bound-and-throwing beats unbound because the error names
the constraint instead of looking like a broken boot, and because
`resolve`/`bound?`/`#'eval`-as-a-value keep matching the REPL.

## 5. Emitter gaps that only compiling core could find

- **Regex literals** had no constant emission (`#"\r?\n"` in
  string.cljg). Now one package-level `&reader.Regex{…}` per literal
  SITE, never deduped: real Clojure's Pattern has no `.equals`, so two
  separately-read `#"same"` are not `=`, while one literal read once IS
  the same object on every evaluation of its form.
- **Dead code after control transfer**: `let`/`if` returned a temp even
  when their body ended in `continue` (recur), emitting an unreachable
  assignment. Harmless until generated code lived in-repo, where
  `go vet`'s unreachable check is a gate. Both now propagate "".
- **`rt` imported unconditionally**: a pure-Clojure package (protocols)
  touches no rt helper, and Go rejects unused imports.

All three are behavior-preserving and covered by the dual harness.

## 6. Verification

- `pkg/coreaot/imports_test.go` — the all-or-nothing link proof
  (`go list -deps`), measured against real symbol counts (pkg/eval
  155 → 0) rather than assumed.
- `pkg/coreaot/generated_test.go` — regenerate + byte-diff: the ONE way
  checked-in generated core can lie is drifting from its sources.
- The dual harness is the semantic proof, unchanged: 221 files compiled
  and byte-compared, 2 waived (eval/macroexpand) with reasons.
