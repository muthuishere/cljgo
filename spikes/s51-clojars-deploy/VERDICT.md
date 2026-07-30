# s51 VERDICT — MET

**Date:** 2026-07-27 · **Spike:** s51-clojars-deploy · **ADR:** 0095 decision 3

## Question

Does a pure-Go Maven deploy (pom + source jar + checksums, Maven layout)
round-trip — publish a pure-Clojure library, consume it back byte-identical?

## Outcome: MET

A `greetlib` pure-Clojure library was built into a Maven artifact, deployed to a
local file-repo in correct layout, and consumed back with the s50 resolver:
**`greetlib/core.clj` recovered byte-identical (190 B), checksums verified,
transitive `org.clojure/tools.cli` recovered from the pom.** All with the Go
standard library — no JVM, no `mvn`, no `lein`.

### What this confirms for ADR 0095

1. **Decision 3 is buildable pure-Go.** pom.xml generation, source-jar zipping,
   sha1/md5 checksums, and Maven path layout are all stdlib. The deferred
   ADR-0054 upload can ship.
2. **gpg signing does not bind.** Clojars accepts unsigned deploys; signing is
   optional detached `.asc` metadata, not a JVM-only gate. Kill condition avoided.
3. **Deploy format == consume format.** The s50 resolver reads a cljgo-deployed
   artifact with zero special-casing — publish and consume are one Maven shape.
4. **Auth is env-only, gated.** `deployHTTP` reads `CLOJARS_USERNAME` /
   `CLOJARS_PASSWORD` (deploy token) at the `PUT`, never logs or bakes them, and
   only fires under `CLOJARS_DEPLOY=1`. The ADR's security line is implementable
   verbatim.

## Residual (apply-time, not a blocker)

- **One live-Clojars `PUT` smoke test**, owner-run with a real deploy token —
  the only step untestable without an account, and the only one with a public
  side effect (it publishes an artifact). Everything up to the socket is proven.
- Optional gpg `.asc` signing if a future consumer requires signed artifacts —
  pure-Go OpenPGP exists; out of scope here.

**Per ADR 0027 §2 this spike is closed.** ADR 0095 decision 3 stands.
