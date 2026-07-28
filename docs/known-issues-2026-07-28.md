# Known issues — found 2026-07-28 (QA sweep + book authoring + spike s66)

Every item below was **reproduced against a real build**, and every JVM claim
was verified against the real `clojure` CLI (1.12.5) — not memory. Nothing here
is fixed yet. These should clear before a version bump.

## P1 — JVM divergences in shipped code

### 1. `clojure.core.match` map patterns break across rows with different key sets
Shipped in the contrib wave (ADR 0097). Repro:

```clojure
(match [e]
  [{:kind :click :button "left"}] "left click"
  [{:kind :click}]                "other click"
  :else                           "unknown")
```

| input | cljgo | JVM core.match 1.1.0 |
|---|---|---|
| `{:kind :click :button "left"}` | `"left click"` | `"left click"` |
| `{:kind :click :button "right"}` | **`"unknown"`** | `"other click"` |
| `{:kind :scroll}` | `"unknown"` | `"unknown"` |

The second row is apparently required to carry `:button` too — the column
key-union handling in the Maranget compiler (`pkg/bri/match.go`) looks wrong.
Each pattern works in isolation; only the multi-row column breaks. **No
conformance file covers this.**

### 2. `for` does not support `:when` / `:while`
```clojure
(for [n (range 1 11) :when (even? n)] n)
```
- cljgo: `error: macroexpanding let: Unsupported binding form: :when`
- JVM: `(2 4 6 8 10)`

`:when`/`:while` are standard `for` modifiers — this is a core gap, and the
natural translation of a Python list comprehension (hit while writing the
"Coming from Python" page).

### 3. Functions cannot carry metadata
```clojure
(with-meta (fn [] 1) {:tag :mock})
```
- cljgo: `error: value of type *eval.evalFn can't have metadata`
- JVM: returns the fn; `(meta f)` → `{:tag :mock}` (fns implement `IObj`)

Found by spike s66. Worked around there with a registry keyed by fn identity.
Owned by the `cljx/fnmeta` work in the ADR 0105 change.

### 4. `extend-protocol` does not resolve fully-qualified class names — FIXED
`java.lang.String` → `No implementation of method: describe of protocol:
user.Describe found for: String`, while bare `String`/`Long` work. Java
arrivals will type the qualified name.

**Fixed 2026-07-28.** `-type-key` now runs a qualified class name through the
ADR 0036 class-ref table down to its simple name — the same reduction
`classDispatchKey` already did for the functional `extend`. Frozen in
`conformance/tests/extend-protocol-qualified-class.clj` (both harnesses).

## P1 — correctness of the test story

### 5. Compiled test binaries exit 0 when tests fail
CI goes green on red. Interpreted `cljgo test` exits 1 correctly. There is also
no `cljgo test --compiled` at all. Owned by the `cljx/runner` work (ADR 0105).

## P2 — error-message quality (error doctrine says these should be better)

### 6. `.getMessage` on a host error fails — FIXED (implemented)
`(try (/ 1 0) (catch Throwable t (.getMessage t)))` → `no method getMessage on
*lang.ArithmeticError`. `ex-message` works. Every JVM-trained user (and LLM)
writes `.getMessage` first — implement it or emit a did-you-mean `Fix`.

**Fixed 2026-07-28 — implemented, not diagnosed.** `.getMessage`,
`.getLocalizedMessage` and `.getCause` are bridged in `CallGoMethod` for ANY
host error receiver (the whole family: `*lang.ArithmeticError`,
`*lang.ExceptionInfo`, plain Go errors from interop), delegating to the same
accessors `ex-message`/`ex-cause` use, so the two spellings cannot disagree in
either mode. `.toString` is deliberately NOT bridged — the JVM answers
`java.lang.ArithmeticException: Divide by zero`, a class name cljgo has no
honest equivalent for. Frozen in `conformance/tests/throwable-get-message.clj`
(both harnesses), all eight rows oracled against clojure 1.12.5.

### 7. `Integer/parseInt` unresolved AND misdiagnosed — FIXED (diagnosed)
→ `error: no such namespace: Integer`. It is a class, not a namespace, so the
diagnosis is wrong. `parse-long` works. THE idiom in every `tools.cli` example
— deserves a `Fix` pointing at `parse-long`.

**Fixed 2026-07-28 — diagnosed, not implemented.** Making it WORK would mean
emulating `java.lang.*` statics on a Go host: a whole feature (ADR-sized), and
one the precedence principle does not require. So the error now says what the
thing is, carries the new **I4001** code + explain page, and fires a
did-you-mean `Fix` for the statics with a real clojure.core replacement:

```
error: no such namespace: Integer (Integer is a Java class, not a namespace:
cljgo hosts Clojure on Go, so the Java static Integer/parseInt is unavailable)
at demo.clj:1:11
help: did you mean parse-long?
help: run `cljgo explain I4001`
```

The leading `no such namespace: X` clause is deliberately kept — it is frozen
by `conformance/tests/java-static-loud-error.clj` and asserted by ADR 0054's
decision-4 test. Frozen in
`conformance/tests/java-static-class-not-namespace.clj`. NOTE: `cljgo build`
still renders only the bare message with no `help:` lines — that is issue 8
below (the build-phase renderer), untouched here.

### 8. Build-time arity error drops fields the run-time one has — FIXED 2026-07-28
Same file, same call: `cljgo run` gives
`wrong number of args (3) passed to: e2/f (expects 1: [x]) at err2.clj:3:1` +
`help:`; `cljgo build` gives only `wrong number of args (3) passed to: e2/f`.
The build-phase renderer skips expected/location/help.

Fixed: `pkg/emit.CompileError` marks the errors that came out of the user's
Clojure, and `cljgo build` routes exactly those through the one shared
`diag.Render` — infrastructure failures (missing file, `go build`) keep their
plain text, as `cljgo run` already did. Frozen by
`cmd/cljgo/build_error_parity_test.go`.

The same sweep closed a second, unlisted divergence in the SAME error: an
arity error that merely UNWOUND through an outer call was re-labelled with
that call's callee, so `cljgo run` blamed `clojure.core/println` for
`(println (map h [1] [2] [3]))` while `cljgo build` and the compiled binary
blamed `user/h` — which is what the JVM says. `pkg/eval`'s call-site
enrichment now only re-labels an error its OWN callee raised
(`conformance/tests/arity-error-nested-call-names-raiser.clj`,
`pkg/repl/arity_naming_test.go`).

### 9. `cljg.data.cast/exec!` params are varargs; the vector form fails opaquely — FIXED 2026-07-28
`(db/exec! conn "insert … values (?)" ["x"])` → `unsupported type lang.Vector,
a struct` (G5007). The vector is the natural guess; the error gives no hint.

Fixed: `cljg.data.cast` rejects a collection param at the API boundary with the
new **G5008** diagnostic, naming the verb the user actually called and the
shape it wanted — `cljg.data.cast/exec!: param 1 is a vector — SQL params are
varargs, not a collection (expects [db sql & params], found a vector passed as
one param); spread it with (apply exec! db sql params)`. Frozen in both
harnesses by `cmd/cljgo/testdata/dbparity.cljg`.

## P2 — gaps

### 10. No `sleep`
No `Thread/sleep` (`no such namespace: Thread`) and no equivalent in `core/` or
`cljg.*`. Decide whether it belongs in `cljg.system`.

### 11. No public writer for `binding [*out* …]`
`java.io.StringWriter` is unavailable; capture requires the private
`(clojure.core/-string-writer)`. `with-out-str` works for whole-body capture.
`cljx.test` makes this moot for tests, but the primitive is still missing.

## Quirks (work as designed, but surprise people)

- `clojure.data.csv/write-csv` is `(write-csv data & opts) → String`, not
  upstream's `(write-csv writer data & opts)`; upstream-shaped calls die with
  `invalid map. must have even number of inputs`. Deliberate + documented in
  `core/data_csv.cljg`, but every JVM snippet breaks.
- `cljgo run file.clj` does not call `-main`; the built binary does.
- Sets have no iteration order (correct, but book examples must not imply one).
- A dev-built `cljgo` needs `CLJGO_SRC` set for `cljgo build`.
