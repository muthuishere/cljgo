# s51 — Clojars/Maven deploy round-trip (ADR 0095 decision 3)

**Falsifiable question.** Does a **pure-Go** Maven deploy — generated `pom.xml`
+ a source-bearing `.jar` + `.sha1`/`.md5` checksums, in the correct Maven
repository layout — round-trip? Publish a pure-Clojure library, then consume it
back and recover byte-identical source?

**Kill condition.** Clojars' deploy protocol needs JVM-only tooling (gpg signing
we can't do shell-free, `maven-metadata.xml` races) that breaks the pure-Go
constraint, OR a self-deployed jar can't be consumed back.

## Safety

This spike deploys to a **local temp file-repo** and consumes it back. It does
**not** push to public Clojars. The authenticated HTTP `PUT` to clojars.org is
implemented (`deployHTTP`) but **gated behind `CLOJARS_DEPLOY=1` + credentials in
the environment**, so a normal `go run .` never publishes a public artifact and
never bakes a secret — credentials are read from `CLOJARS_USERNAME` /
`CLOJARS_PASSWORD` (a deploy token) at the point of the `PUT` and never printed.

## Run

```
cd spikes/s51-clojars-deploy
go run .        # local round-trip only; no network, no public push
```

Stdlib only (`archive/zip`, `crypto/sha1`, `crypto/md5`, `encoding/xml`,
`net/http`).

## What it does

1. **Build** a Maven artifact from a pure-Clojure `greetlib` source tree: a
   `pom.xml` (coordinate + a pure transitive dep, `org.clojure/tools.cli`) and a
   source-bearing `.jar` (the `.clj` payload a JVM consumer compiles — ADR 0054).
2. **Checksum** each with `.sha1` + `.md5` (Maven requires them alongside).
3. **Deploy** into a local repo dir in the exact Maven path layout
   (`io/github/muthuishere/greetlib/0.1.0/greetlib-0.1.0.{jar,pom}` + checksums).
4. **Consume it back** (s50-style): read + checksum-verify the pom, parse its
   transitive deps, read + verify the jar, extract source.
5. **Assert** the recovered source is byte-identical and the transitive dep
   round-trips through the pom.

## Result (2026-07-27, `results.txt`)

- Artifact built pure-Go: pom 479 B, source jar 301 B.
- 6 files written in correct Maven layout (jar/pom × {—, .sha1, .md5}).
- Consumed back: **`greetlib/core.clj` byte-identical (190 B)**, checksums
  verified, transitive `org.clojure/tools.cli 1.1.230` recovered from the pom.
- **VERDICT: ROUND-TRIP MET.**

## Findings

- **Kill condition NOT triggered.** The whole deploy artifact — pom, source jar,
  checksums, Maven layout — is generated with the Go standard library. No JVM, no
  `mvn`, no `lein`.
- **gpg signing is not required to deploy to Clojars** (it is optional metadata;
  Clojars accepts unsigned pushes), so the one plausibly-JVM-only step doesn't
  bind. If signing is later wanted, it is detached `.asc` files alongside — a
  pure-Go OpenPGP path exists, but is out of this spike's scope.
- **The public `PUT` is trivial and isolated:** 6 authenticated requests,
  mechanically identical to the local writes proven here. It is the only part
  that can't be unit-tested without a real account, so it is gated and left for a
  human-run, credentialed check at apply time.
- **Consume-what-we-publish holds:** the s50 resolver reads a cljgo-deployed
  artifact with no special-casing — the deploy format and the consume format are
  the same Maven shape.

## Verdict

See `VERDICT.md`: **MET.** The pure-Go deploy round-trips; ADR 0095 decision 3
stands. The only apply-time residual is a one-time credentialed live-Clojars
`PUT` smoke test (owner-run, since it publishes publicly).
