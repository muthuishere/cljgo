# ADR 0062 — Internationalisation (bri.i18n)
Date: 2026-07-23 · Status: accepted (owner-directed, 2026-07-23; evidence: spike
S41) · New bri battery; additive (Clojure core has no i18n — no JVM oracle,
judged on design soundness + a working prototype).

## Context

The owner wants Java-style i18n — locale message bundles, interpolation,
plurals. Spike S41 built a pure-Go, static-embeddable prototype passing 17/17
assertions.

## Decision

1. **Bundles in `.edn` (canonical) and `.properties` (Java-familiar skin)** —
   the same call as ADR 0059 — resolving through one lookup; a `:greeting`
   keyword and a `greeting=` line collapse to the same key.
2. **Locale resolution + fallback:** `en_US → en → default` (Java ResourceBundle
   model). A missing key returns a **visible marker `⟦missing:key⟧`** by default
   (never a panic — the one behavior that must not crash a handler); `:strict?`
   opt-in throws for prod.
3. **Interpolation:** named args — `(t :greeting {:name "Muthu"})` → `"Hello,
   Muthu!"`; an unsupplied `{name}` stays visible, not blanked.
4. **Plurals:** bind pure-Go **`golang.org/x/text/feature/plural`** (full CLDR
   categories) behind `(t key {:count n})`, replacing the spike's hand-rolled
   en/fr 3-category engine. EDN plural maps `{:one … :other …}` and inline
   ICU-subset `{count, plural, …}` both supported; a pluggable rule hook is the
   fallback. **The `x/text` bind is chosen but un-exercised** — S41 proved only
   the hand-rolled path (see Un-proven).
5. **Comptime-embedded default bundles** (ADR 0021 embed) + a **disk override
   dir** merged per-key: the app ships its locales inside the single binary,
   overrides live on disk. Locale precedence: `(with-locale :fr …)` > HTTP
   `Accept-Language` > config `APP_LOCALE` > `en` default.
6. **`bri.i18n` API:** `(t key)`, `(t key args)`, `(t locale key args)`,
   `(with-locale … )`, `(load-bundles path)`. `t` does not shadow clojure.core.

## Consequences

- Apps get first-party i18n **inside the single binary**; `x/text/feature/plural`
  gives real CLDR breadth, pure-Go.
- Un-proven (S41): the `x/text/feature/plural` bind itself (the spike ran a
  hand-rolled en/fr engine); bundle hot-reload / REPL-liveness (loaded once in
  the spike); `.properties` `\uXXXX`/ISO-8859-1 fidelity; `Accept-Language`
  q-value sorting; ADR-0021 comptime-embed parity (the spike used stdlib
  `//go:embed`).
- **Constraint-filter #4 commitment (ADR 0056):** bundles + fallback +
  interpolation land with dual-harness conformance (identical lookups
  interpreted AND AOT-compiled — the one parity i18n owes, absent a JVM oracle).
  No hot-path perf budget: `t` is a map lookup + interpolation, not a loop.
- Not chosen: full ICU MessageFormat (`select`/`selectordinal`/number-date
  skeletons deferred — *data not code*, the engine already keys the full
  category set); `.properties` canonical; a hand-rolled plural table.
