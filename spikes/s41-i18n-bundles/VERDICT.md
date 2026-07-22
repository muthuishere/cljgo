# S41 VERDICT — bri.i18n (locale bundles, interpolation, plurals)

Status: CLOSED. Recommendation: **BLESS `bri.i18n`. A pure-Go,
`CGO_ENABLED=0`, single-binary i18n with embedded `.properties`+`.edn`
bundles, ResourceBundle-style `en_US→en→default` fallback, named-arg
interpolation, and a CLDR-subset plural engine is proven and sound.
Ship the loader + `t` + `with-locale` surface. Two owner calls below.**

Judged on design + a working prototype (Clojure core has no i18n, so
there is no JVM oracle). Reproduce: `./run.sh` — builds a
`CGO_ENABLED=0` static binary with locales embedded, runs 17 assertions.

## Evidence (all 17 PASS)

| crit | assertion | result |
|---|---|---|
| 1 | `.properties` key and `.edn`-only key resolve through the same `T()` | PASS (`greeting` from .properties, `tagline` from messages.edn) |
| 2 | `en_US` → `en` fallback finds a base-only key (`help`) | PASS |
| 2 | most-specific bundle wins (`en_US` `greeting` = "Hi") | PASS |
| 2 | default (suffix-less) bundle reached as last resort (`app.name`) | PASS |
| 2 | missing key → visible `⟦missing:key⟧`, no crash | PASS |
| 3 | `(t :greeting {:name "Muthu"})` → `Hello, Muthu!` | PASS |
| 3 | unsupplied placeholder stays visible (`Hello, {name}!`) | PASS |
| 3 | EDN-bundle interpolation (fr) | PASS |
| 4a | inline ICU-subset plural `{count, plural, …}`, count 0/1/5, `#`→count | PASS |
| 4b | EDN plural-map `{:one … :other …}`, fr rule (0 and 1 → one), count 0/1/5 | PASS |
| 5 | disk override shadows an embedded key AND adds a brand-new key | PASS |
| 6 | locale resolver precedence explicit > Accept-Language > config > default | PASS (sketch) |

## Positions per exit criterion

**1 — two formats, one lookup: PROVEN, keep both.** A ~35-line
`java.util.Properties` subset parser (`#`/`!` comments, `=`/`:`
separators, `\` continuation) and an EDN decoder
(`olympos.io/encoding/edn`, the same dep s38's config battery already
pulls) feed one `Bundle map[string]Message`. `.edn` is canonical
(consistent with s38); `.properties` is the Java-familiar on-ramp. An
EDN keyword `:greeting` and a properties `greeting=` collapse to the
same lookup key by construction. Full `.properties` fidelity
(`\uXXXX`, ISO-8859-1) would swap in `github.com/magiconair/properties`
(pure Go) — named, not built.

**2 — fallback: PROVEN, matches ResourceBundle.** `localeChain("en_US")`
= `["en_US","en",""]`, searched most-specific-first; the language
subtag (`en`) drives plural rules. A miss returns `⟦missing:key⟧`, the
one behavior that must never be a panic in a request handler.

**3 — interpolation: PROVEN.** `{name}` from a map; an unknown
placeholder is left literally visible rather than blanked, so a missing
arg is diagnosable. This is `MessageFormat`'s named-argument idea
without its `'{0}'` positional/quoting baggage.

**4 — plurals: PROVEN for the demo locales; a deliberate ICU SUBSET.**
Both authoring shapes work and select through one CLDR category
function: inline `{count, plural, zero{…} one{# item} other{# items}}`
in `.properties`, and `{:one … :other …}` maps in `.edn`. `#`
substitutes the count (ICU semantics). Distance from real ICU stated
below.

**5 — embed + override: PROVEN.** `//go:embed locales` bakes every
bundle into the binary (this is exactly cljgo's comptime `embed`, ADR
0021 — same mechanism, so the single-binary claim holds). A disk
`override/` dir is merged on top at load, shadowing embedded keys
per-key and adding new ones. So: ship-with-defaults, let-apps-override.

**6 — locale source: SKETCHED, precedence set.** `resolveLocale`
returns the first non-empty of: explicit `with-locale` arg → HTTP
`Accept-Language` (first tag, `en-US`→`en_US`) → config `APP_LOCALE`
→ built-in default. `with-locale` binds a dynamic var; the web layer
supplies Accept-Language per request.

## How close to real ICU we landed

Close on the *shape*, deliberately short on *coverage*:

- **Categories:** we implement `zero/one/other`; real CLDR has six
  (`zero one two few many other`). The engine already keys on the full
  keyword set — adding `few`/`many` is data, not code.
- **Rules:** hand-rolled `en` and `fr` rules only. Real CLDR derives
  per-language rules from operands `n,i,v,w,f,t`
  (https://cldr.unicode.org/index/cldr-spec/plural-rules). Production
  path: bind `golang.org/x/text/feature/plural` (pure Go, CLDR-backed,
  `CGO_ENABLED=0`-clean) OR vendor a generated table. The plural rule is
  a single pluggable `func(int) PluralCategory` — swapping the source is
  local.
- **Not implemented (named ICU gaps):** `select`/`selectordinal`,
  number/date/currency skeletons, gendered/nested arguments, `'{'`
  literal quoting, and q-value sorting of `Accept-Language`. A bri that
  wants full ICU should bind `x/text` rather than grow this parser.

Reference: ICU MessageFormat
(https://unicode-org.github.io/icu/userguide/format_parse/messages/).

## Blessed `bri.i18n` API (see `shapes/bri-i18n.cljg`)

- `(t key)` · `(t key args)` · `(t locale key args)` — translate; `args`
  is a map, `:count` is the plural operand. Missing key → visible marker,
  never throws.
- `(with-locale :fr … )` — dynamic-var scope, highest-precedence source.
- `(load-bundles path)` — layer a disk dir over the embedded set.
- `messages` — the comptime-embedded default set (ADR 0021 `^:embed`).

`t` does not collide with clojure.core (precedence principle holds); it
is the conventional i18n verb.

## What was NOT proven (un-proven risks)

1. **REPL liveness of bundles.** s25/s37 proved live re-`def`; here the
   store is loaded once. Whether editing a `.properties` file
   hot-reloads through the running evaluator (file-watcher vs explicit
   `(load-bundles)`) is unproven — likely an explicit reload verb, but
   untested.
2. **Real CLDR breadth.** Only `en`/`fr` and 3 categories exercised.
   Arabic (6 categories), Polish (few/many), Russian, etc. are asserted-
   by-design, not run. The `x/text/feature/plural` bind is the de-risk.
3. **`.properties` edge fidelity.** `\uXXXX`, ISO-8859-1, multi-line
   escapes not implemented (common subset only).
4. **Accept-Language q-sorting** — we take the left-most tag, not the
   highest-q.
5. **Interop with cljgo's actual embed comptime** — used stdlib
   `//go:embed`; ADR 0021 parity assumed, not wired.

## Owner-gated questions

1. **Plural-model depth.** Three options:
   - (a) **Ship the 3-category hand-rolled engine** (this spike) —
     zero new deps, covers en/fr/most-Western; grow the table as
     locales are added.
   - (b) **Bind `golang.org/x/text/feature/plural`** — full CLDR, pure
     Go, `CGO_ENABLED=0`-clean, ~one indirect dep; correct for all ~200
     locales out of the gate.
   - (c) Full ICU MessageFormat (`select`, skeletons, nesting) — heavy,
     probably a downstream library, not the blessing.
   **Recommendation: (b)** — CLDR correctness for a dep that is already
   Go-team-maintained and CGO-free is the right default for an i18n
   battery; keep the engine's pluggable rule hook so (a) stays a
   fallback for locales x/text lacks.

2. **Locale-resolution default.** When nothing is specified, what is the
   process default — hard-coded `en`, or config `APP_LOCALE` required?
   **Recommendation:** default to `en` but honour `APP_LOCALE` when set,
   and in web scope let `Accept-Language` win per-request (the sketched
   precedence). Confirm `en` is the acceptable built-in floor.

3. **Missing-key policy in prod.** Visible `⟦missing:key⟧` is right for
   dev; some shops want strict mode (fail the build / log-and-return-key)
   in prod. Ship the visible marker as default; add a `:strict?` switch?
   (Recommend: marker default, strict opt-in.)
