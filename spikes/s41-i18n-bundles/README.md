# S41 — bri.i18n: locale message bundles, interpolation, plurals

Clojure core has **no** i18n. This is a pure ADDITION: there is no JVM
oracle to match, so the spike is judged on design soundness + a working
prototype, and it cites Java `ResourceBundle` / `.properties` and ICU
`MessageFormat` semantics where it leans on them.

The owner wants Java-style i18n done the bri way: embeddable locale
message bundles, named-argument interpolation, and pluralization —
under `bri.i18n`, never shadowing clojure.core (the precedence
principle), consistent with the config battery decision (**`.edn`
canonical, `.properties` accepted for Java familiarity** — s38).

## The one question

Can a single pure-Go static binary carry its locales (embedded), resolve
`en_US → en → default` with a visible-not-crashing miss, interpolate
named args, and pluralize with a pragmatic CLDR/ICU subset — and what is
the blessed `bri.i18n` API on top of it?

## Exit criteria (written before any code)

1. **Two bundle formats, one lookup.** `messages.properties`
   (`greeting=Hello, {name}!`) AND `messages.edn`
   (`{:greeting "Hello, {name}!"}`) resolve through the SAME lookup and
   return the same value. Java `.properties` = ISO-8859-1 `key=value`
   with `#`/`!` comments and `\` continuations (java.util.Properties);
   `.edn` = a map of keyword→string. Both proven.
2. **Locale resolution + fallback chain.** ResourceBundle-style filename
   suffixes (`messages_en_US`, `messages_en`, `messages`). A key present
   ONLY in base `en` is found when the locale is `en_US`; a key present
   in NO bundle returns a **visible marker** (`⟦missing:key⟧`), never a
   crash.
3. **Interpolation.** Named args `{name}` filled from a map:
   `(t :greeting {:name "Muthu"})` → `"Hello, Muthu!"`. Unsupplied
   placeholders are left visible, not blanked.
4. **Pluralization.** A CLDR/ICU-style category select (zero/one/other).
   BOTH shapes proven: (a) inline ICU-subset string
   `{count, plural, one {# item} other {# items}}`, and (b) an EDN
   plural map `{:one "1 item" :other "{count} items"}`.
   `(t :items {:count 1})` vs `{:count 5}`. Distance from real ICU
   stated explicitly.
5. **Comptime embed + disk override.** Bundles loaded from an embedded
   `embed.FS` (single binary carries locales) AND an override directory
   on disk that shadows/extends the embedded set at runtime.
6. **Locale resolution source.** How the current locale is chosen —
   config `APP_LOCALE`, HTTP `Accept-Language`, or an explicit
   `with-locale` arg — sketched with a resolver + precedence.
7. **VERDICT.md** takes a position, states what was NOT proven, and
   routes the genuine owner calls (plural-model depth, locale-resolution
   default) to the owner with options + a recommendation.

## Non-goals

Not building `bri.i18n`. Not touching `pkg/` / `core/` / `cmd/` /
conformance / root go.mod. No full ICU MessageFormat (number/date/select
skeletons, gender, nested args) — a subset, with the gap named. No CLDR
plural-rule table for all ~200 locales — English + a pluggable rule hook,
with the real source cited. Probe code is throwaway (ADR 0027).

## Layout

- `locales/` — embedded bundles (`.properties` + `.edn`, several
  locales) baked into the binary via `embed.FS`.
- `override/` — an on-disk bundle that shadows an embedded key at
  runtime (proves criterion 5's override path).
- `*.go` — the loader (properties + edn parsers), the fallback resolver,
  the interpolator, the plural engine, and a `main.go` proof harness
  printing PASS/FAIL per criterion.
- `shapes/bri-i18n.cljg` — the blessed `bri.i18n` `.cljg` API sketch.
- `run.sh` — `go run .`, prints PASS/FAIL per exit criterion.
- `VERDICT.md` — position + evidence + owner calls.
