# cljgo munging scheme (public contract from M2)

How Clojure names become Go identifiers in emitted code. Per ADR 0013
(every cljgo project is a first-class library) this is a **public,
stable contract**: Go programs will eventually import emitted packages
by these names, so changes after M2 are breaking changes and need an
ADR that supersedes this scheme.

## Identifier munging

Applied to a single Clojure name segment (a var name, a namespace name):

1. `[A-Za-z0-9_]` pass through unchanged.
2. Characters with a JVM-Clojure `Compiler.CHAR_MAP` token use that
   token, so names look familiar across Clojure implementations:

   | char | token | char | token |
   |---|---|---|---|
   | `-` | `_` | `!` | `_BANG_` |
   | `:` | `_COLON_` | `@` | `_CIRCA_` |
   | `+` | `_PLUS_` | `#` | `_SHARP_` |
   | `>` | `_GT_` | `'` | `_SINGLEQUOTE_` |
   | `<` | `_LT_` | `"` | `_DOUBLEQUOTE_` |
   | `=` | `_EQ_` | `%` | `_PERCENT_` |
   | `~` | `_TILDE_` | `^` | `_CARET_` |
   | `&` | `_AMP_` | `*` | `_STAR_` |
   | `|` | `_BAR_` | `?` | `_QMARK_` |
   | `{` `}` | `_LBRACE_` `_RBRACE_` | `[` `]` | `_LBRACK_` `_RBRACK_` |
   | `/` | `_SLASH_` | `\` | `_BSLASH_` |

3. **cljgo extensions** (Go needs a total mapping; the JVM leaves these
   to class-file naming):
   - `.` → `_DOT_` (Go identifiers cannot contain dots).
   - Any other rune → `_uXXXX_` (lowercase hex of the code point), so
     the mapping is total over Unicode.
4. A result starting with a digit or `_` is prefixed with `X` (a Go
   identifier can't start with a digit, and a leading `_` would make
   the name unexportable — cljs2go's rule). This applies AFTER step
   2/3 tokens too, uniformly: `-main` → `X_main`, `+` → `X_PLUS_`,
   `*ns*` → `X_STAR_ns_STAR_`.
5. A result equal to a Go keyword or predeclared identifier gets a
   trailing `_` (e.g. `map` → `map_`).

The mapping is deliberately **not injective** (`a-b` and `a_b` both
munge to `a_b` — same trade-off JVM Clojure makes). Within one emitted
file the emitter deduplicates minted identifiers with a numeric suffix;
across the public library surface (M2+ `--lib`), colliding sibling
names are a compile-time error to be surfaced by the emitter.

## Namespace → package mapping (design/04 §1)

`my-app.core` → directory `my_app/core`, Go package name `core`
(`.` → `/`, then each segment munged as above). v0 emits a single
`main` package; the mapping applies from multi-namespace support (v0.5)
but is fixed now as part of the contract.

## Emitted-file naming conventions (internal, not part of the contract)

Hoisted package-level interns are prefixed to avoid colliding with each
other and with locals: `v_` + munged `ns/name` for vars, `kw_` + munged
full name for keywords, `sym_` for quoted symbols. Locals and temps get
a per-file monotonic numeric suffix (`x3`, `tmp7`), which makes
shadowing collision-free by construction (S1/S5-proven). These interior
names may change without notice; only exported names (none in v0) and
the rules above are the contract.
