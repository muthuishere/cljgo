# tasks — adr-0113-date-patterns

## 1. The pattern compiler (Go host side)
- [ ] 1.1 Port `spikes/s71-date-patterns/{pattern.go,direct.go}` into `pkg/corelib` as an op-list compiler. Layout-string translation is NOT ported.
- [ ] 1.2 Refusals carry a registered diagnostic naming the token; `docs/diagnostics/<CODE>.md`; regenerate the registry lock.
- [ ] 1.3 Memoise compiled patterns; race-free under concurrent use.

## 2. The Clojure surface
- [ ] 2.1 `cljg.date/format-pattern` and `parse-pattern`, docstrings stating Locale.ENGLISH explicitly and NOT Locale.ROOT.
- [ ] 2.2 Existing `format`/`parse` unchanged; docstrings point at the portable choice.

## 3. Differential evidence
- [ ] 3.1 Commit the 4,000-pattern corpus + `oracle.clj`; a test asserts 0 divergences and 0 accepted-what-JVM-rejects.
- [ ] 3.2 Conformance file with frozen `;; expect:` output, oracle-cited, running under both harnesses.

## 4. Gates
- [ ] 4.1 Full gate green; dual-harness parity (REPL vs compiled) on the conformance file.
