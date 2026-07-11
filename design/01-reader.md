# 01 — The Reader (text → Clojure data)

Component of the Go-hosted Clojure AOT compiler. The reader turns UTF-8 text into
our runtime's persistent data structures (`pkg/lang`), with position metadata, so the
analyzer/compiler downstream can report errors and emit Go with source mapping.

References studied: `clojure.lang.LispReader` (1702 loc), `clojure.lang.EdnReader`,
Glojure `pkg/reader` (1434 loc), let-go `pkg/compiler/reader.go` (1580 loc).

Guiding rule: **the reader is dumb and faithful**. It produces data, never evaluates
(except syntax-quote expansion, which is a pure data→data transform done at read time,
exactly as Clojure does). Everything namespace-y goes through an injected `Resolver`
so the reader package has no dependency on the compiler.

---

## 1. Scope checklist, ordered by implementation phase

### Phase 0 — MVP (enough to read real defn-level code)
- [ ] whitespace = Unicode space + `,`; line comments `;` and `#!`
- [ ] `nil` `true` `false`
- [ ] symbols (incl. `/`, `foo/bar`, validation per `symbolPat`)
- [ ] keywords `:foo`, `:ns/foo` (NOT auto-resolved `::` yet)
- [ ] integers: decimal, `0x` hex, `0` octal, `NrDIGITS` radix (2–36), `N` bigint suffix, `+`/`-` sign
- [ ] floats + exponent, `M` BigDecimal suffix; ratios `1/3`
- [ ] strings with Clojure escapes only: `\t \r \n \\ \" \b \f \uXXXX \o000–\o377`
- [ ] character literals: `\a`, `\newline \space \tab \backspace \formfeed \return`, `\uXXXX`, `\oNNN`
- [ ] lists `()`, vectors `[]`, maps `{}` (even-forms check), unmatched-delimiter errors
- [ ] sets `#{}` (duplicate-key error)
- [ ] `'form` → `(quote form)`; `@form` → `(clojure.core/deref form)`
- [ ] `#_` discard (incl. stacked `#_#_`)
- [ ] `^meta` (map, keyword→`{:kw true}`, symbol/string→`{:tag v}`, vector→`{:param-tags v}`), merge onto following form
- [ ] line/col/end positions on all collections + symbols; positioned errors

### Phase 1 — macro-writing support
- [ ] `` ` `` syntax-quote, `~` unquote, `~@` unquote-splicing (see §2)
- [ ] auto-gensym `x#` inside syntax-quote (global counter)
- [ ] `#()` fn shorthand with `%`, `%1..%n`, `%&` (nested `#()` is an error; args named `pN__<id>#` — trailing `#` is deliberate, it keeps `` `#(...) `` hygienic)
- [ ] `#'x` → `(var x)`
- [ ] `#"..."` regex (raw string capture; see §4 for engine decision)
- [ ] `##Inf ##-Inf ##NaN`
- [ ] `#^` legacy meta (alias of `^`)

### Phase 2 — full fidelity
- [ ] auto-resolved keywords `::foo`, `::alias/foo` (needs Resolver)
- [ ] namespaced maps `#:ns{...}`, `#::{...}`, `#::alias{...}`, `_` ns-escape key
- [ ] tagged literals `#inst #uuid #mytag form` via pluggable data-readers table; unknown tag → error (or default-reader hook); tag containing `.` = record ctor syntax → **compile-time construct, hand to analyzer as TaggedLiteral**
- [ ] reader conditionals `#?(...)` `#?@(...)`: `:allow` / `:preserve` modes, feature set `#{:default :gljgo}` (we claim our own platform key), suppressed read of non-matching branches, splice via pending-forms queue, no splice at top level
- [ ] `#=` eval reader → always an error in AOT mode (no runtime to eval into)
- [ ] `#<` unreadable → error
- [ ] array-class symbols `String/1` (`arraySymbolPat`) — parse, map to Go slice types later

Out of scope forever: `*read-eval*` machinery (AOT: `#=` rejected), `#!` as shebang is just a comment.

---

## 2. Syntax-quote expansion (the hard part)

Done **at read time**, exactly like `LispReader.SyntaxQuoteReader`: `` `form `` reads
`form` fully, then rewrites it into code that *reconstructs* it, resolving symbols.

Algorithm `syntaxQuote(form)`:

1. **Special symbol** (`def`, `if`, `fn*`, `&`, …) → `(quote form)` untouched.
2. **Symbol:**
   - `x#` (no ns, `#` suffix): look up in the **gensym env** — a map created fresh per
     *outermost* `` ` `` read; miss → intern `x__<nextID>__auto__`. Same `x#` twice in
     one syntax-quote = same gensym; different syntax-quotes = different. `x#` outside
     `` ` `` = error ("Gensym literal not in syntax-quote").
   - `Foo.` (ctor): resolve `Foo` via Resolver, re-append `.`.
   - `.foo` (method): leave as-is.
   - otherwise resolve: qualified → resolve ns part as alias/type; unqualified →
     resolve as type, then var; unresolvable → qualify with `resolver.CurrentNS()`.
   - result wrapped: `(quote resolved-sym)`.
3. **`(unquote x)`** → `x`. **`(unquote-splicing x)`** here (not inside a collection
   walk) → error "splice not in list".
4. **Collections** — rewrite via `sqExpandList` over elements, where each element
   becomes `(list (syntaxQuote el))`, unquote → `(list x)`, splicing → `x`, then:
   - seq/list: `(clojure.core/seq (clojure.core/concat ...))`; empty → `(clojure.core/list)`
   - vector: `(clojure.core/apply clojure.core/vector (seq (concat ...)))`
   - map (flatten k/v first): `(apply hash-map (seq (concat ...)))`
   - set: `(apply hash-set (seq (concat ...)))`
5. **Keyword / number / char / string / bool / nil** → itself (self-evaluating).
6. Anything else → `(quote form)`.
7. **Metadata:** if `form` carries meta beyond our position keys, wrap result:
   `(clojure.core/with-meta ret (syntaxQuote meta-minus-line-col))`. Glojure skips
   this; we must not — `^:private` inside a macro template matters.

Nested `` ` `` needs no special code: the inner backtick is *read* while reading the
outer form, so it recursively expands first with its own gensym env (matches Clojure's
push/pop of `GENSYM_ENV`). Depth-limit the recursion (~64) — Glojure's fuzzer found
exponential blowup.

**Gensym IDs come from one atomic global counter** shared with `clojure.core/gensym`
(`NextID func() int64` option, default package-level `atomic.Int64`). Glojure uses
`len(symbolNameMap)` — two separate syntax-quotes both mint `x__0__auto__`, breaking
hygiene when expansions meet. We copy Clojure (`RT.nextID()`), not Glojure.

---

## 3. Go package design

```
pkg/reader/
  scanner.go      // position-tracking rune scanner (1-rune unread)
  reader.go       // Reader, dispatch tables, collections, tokens
  string.go       // string/char/regex readers (Clojure escape rules, not Go's)
  number.go       // matchNumber port of intPat/floatPat/ratioPat
  syntaxquote.go  // §2
  cond.go         // reader conditionals + pending-forms queue
  error.go
```

Depends only on `pkg/lang` (values: Symbol, Keyword, PersistentList/Vector/Map/Set,
Char, BigInt, BigDecimal, Ratio, IObj/IMeta) — never on analyzer/compiler.

```go
// scanner.go — copy Glojure's trackingRuneScanner shape
type Position struct {
    File      string
    Line, Col int // 1-based
}

type scanner struct {
    rs       io.RuneScanner
    file     string
    next     Position    // position of next rune
    history  [2]Position // for one-rune Unread
}
func (s *scanner) Read() (rune, error)
func (s *scanner) Unread()
func (s *scanner) Pos() Position

// error.go
type Error struct {
    Pos   Position
    Start *Position // set for "EOF while reading, starting at line N"
    Err   error
}
func (e *Error) Error() string // "file:line:col: msg"
func (e *Error) Unwrap() error
var ErrEOF = errors.New("EOF") // clean end-of-input, NOT a malformed form

// resolver — mirror of LispReader.Resolver, injected by compiler
type Resolver interface {
    CurrentNS() *lang.Symbol
    ResolveAlias(sym *lang.Symbol) *lang.Symbol // ns aliases
    ResolveVar(sym *lang.Symbol) *lang.Symbol
    ResolveType(sym *lang.Symbol) *lang.Symbol  // "class" → Go type name
}

type ReadCondMode int // CondOff | CondAllow | CondPreserve

type Reader struct {
    s            *scanner
    resolver     Resolver
    features     map[string]bool          // {"gljgo","default"} always
    readCond     ReadCondMode
    dataReaders  map[string]DataReaderFn  // func(form any) (any, error)
    nextID       func() int64
    fnArgs       map[int]*lang.Symbol     // non-nil only inside #()
    pending      []any                    // #?@ splice queue
    sqDepth      int
}

func New(r io.RuneScanner, opts ...Option) *Reader
func (r *Reader) ReadOne() (any, error)  // ErrEOF at clean end
func (r *Reader) ReadAll() ([]any, error)
```

**Dispatch:** a plain `switch` on the rune inside `read()` (Glojure style), not
Clojure's `IFn[256]` table — Go has no reason for indirection, and runes exceed 256.
Same for the `#` sub-dispatch. `isTerminatingMacro` set: everything in
`()[]{}"';^@`~\\%` except `#'%`.

**Collection reading:** one `readDelimited(end rune) ([]any, error)` used by
list/vector/map/set/ns-map. It records the start `Position` so unterminated
collections report *"EOF while reading, starting at line N"* — copy Clojure, this is
the single most useful reader error.

**Position metadata:** push `Pos()` before reading a form, pop after; if the result
implements `lang.IObj`, assoc `:file :line :column :end-line :end-column` (Glojure's
`posStack` approach — better than Clojure, which only stamps lists). Primitives
(numbers, strings, keywords) can't carry meta — the analyzer inherits the enclosing
form's position for them, which is fine in practice. Syntax-quote strips these keys
before comparing "does user meta remain" (§2.7). Keys live in `pkg/lang` so the
compiler shares them.

**Pending-forms queue:** `read()` drains `r.pending` before touching the scanner —
this is how `#?@(:gljgo [1 2 3])` yields `1` then queues `2 3` (Clojure's
`pendingForms` LinkedList, Glojure's `pendingForms` slice; identical trick).

---

## 4. Glojure's reader: keep vs redo

**Copy (it's good):**
- `trackingRuneScanner` + pos history for unread; `posStack` → start/end meta on every IObj.
- Functional options (`WithFilename`, `WithResolver`...); `ErrEOF` sentinel contract
  ("EOF = input exhausted, never malformed form") — excellent for REPLs.
- Pending-forms slice for conditional splicing; syntax-quote depth limit (fuzz-found DoS).
- Overall shape: switch dispatch, `readForColl`, small per-form methods.

**Redo (it's wrong or un-Clojure):**
- **Strings:** Glojure hand-collects then calls `strconv.Unquote` — Go escapes ≠
  Clojure escapes (Go accepts `\a \v \x`, Clojure doesn't; octal ranges differ). Port
  `StringReader`'s switch verbatim.
- **Gensym counter** per-map-length hygiene bug (§2). Use global atomic.
- **Keywords:** Glojure ns-qualifies *every* plain keyword with current ns (`:foo` →
  `:user/foo`!) and doesn't do `::alias/k`. Ours: `:foo` stays plain; `::` resolves
  via Resolver; reject `:foo:bar`, `::` alone, keyword starting with digit per `symbolPat`.
- **Number detection:** Glojure regex-checks the accumulating token *every rune*
  inside `readSymbol` (O(n²), hacky). Do it Clojure's way: peek first char (digit, or
  `+`/`-` followed by digit) → `readNumber`, else token → `interpretToken`.
- **Symbol validation:** Glojure `recover()`s a panic from `NewSymbol`. Validate with
  the ported `symbolPat` rules (no `::` interior, no trailing `:`/`/`...) and return errors.
- **Tagged literals:** Glojure falls back to `[tag form]` vector — silently wrong.
  Unknown tag = error unless a default-reader fn is configured.
- **Syntax-quote meta preservation** missing (§2.7); empty-collection special cases
  divergent from Clojure — port `SyntaxQuoteReader` faithfully instead.
- **Reader conditionals:** Glojure hardcodes `:glj`/`:default` and fully parses
  non-matching branches. We inject the feature set and (v2) read unmatched branches in
  suppressed mode; still simpler than Clojure's re-entrant `readCondDelimited`.
- **Regex:** decision — store the **raw pattern string** in a `lang.Regex{Pattern string}`
  value at read time; compile lazily at runtime with `regexp` (RE2) and document the
  Java-regex gaps (no backreferences/lookbehind). Compiling in the reader (Glojure)
  makes reading fail on patterns the program may never run.
- String building with `+=` → use `strings.Builder` everywhere.

From let-go worth stealing later: its optional token stream (`NewLispReaderTokenizing`)
for editor tooling, and `skipReaderForm` (textual skip for unmatched cond branches).
Not v0.

---

## 5. Milestones

**v0 (Phase 0 checklist)** — accepts and structurally round-trips:

```clojure
(defn add [a b] (+ a b))                       ; lists, vectors, symbols
{:name "muthu" :tags #{:go :clj} :n 42}        ; map, set, keyword, string, int
[1 -2.5 3/4 0xff 2r1010 36rZZ 12N 1.0M \a \newline]
'(a b) @state ^:private x ^String s #_(ignored) 
(str "esc\tA\377")                        ; Clojure escape set
; comment, and #! comment
```
Success check: golden tests reading every form above → printed back via `lang.PrintString`
equals `clojure -M -e '(pr (read-string ...))'` output; unterminated `(` reports
`f.clj:1:1`-anchored "starting at line" error.

**v1** — macros writable: `` `(let [x# ~v] (inc x#)) `` expands to the same tree
Clojure produces (assert against real Clojure's `(read-string "`...")` output, modulo
gensym ids); `#(+ % %2)`, `#'foo`, `#"a+b"`, `##Inf`.

**v2** — `#?(:gljgo x :clj y :default z)`, `#::{:a 1}`, `::alias/k`, `#inst`/`#uuid`
+ custom data readers, `#=` rejected with a clear error. Conformance: run Glojure's
`clj_conformance_test.go` corpus + `refs/glojure/pkg/reader/testdata` against ours.

**v3 (hardening)** — Go fuzz target (`FuzzReadOne`), suppressed-read for cond
branches, array-class symbols, perf pass (Builder, no regex in hot paths).
