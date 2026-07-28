# s67 — docs by-example generator (ADR 0104 spike)

**Verdict: MET.** The Rust-By-Example shape works in the existing
Astro+Starlight site with a tiny generator and zero new dependencies.

Proven here:
- **conformance → page generation**: `gen.go` converts curated
  `conformance/tests/*.clj` into numbered `.md` pages — leading `;;` comment
  block → teaching prose (oracle/QA bookkeeping lines stripped), body → code
  block, `;; expect:` → real output block, plus a "run it yourself" footer.
  Because the sources are frozen conformance tests, every example is CI-run
  in both harnesses on every commit — the doctest guarantee is free.
- **Numbered linear course**: Starlight sidebar group "By example" with
  `sidebar.label: "N. Title"` + `order` gives the mdBook-style numbered nav
  (1. Hello World … 7. Serve HTTP) and built-in prev/next pagination.
- **Human names, namespaces buried**: labels are "Compression", "Passwords",
  "Serve HTTP" — `cljg.compress`/`cljg.security`/`cljg.http` appear only in
  the require line inside the page.
- Verified by full `npm run build` (32 pages) + local serve.

Gotchas for cmd/gendocs (the production home):
- **Use `.md`, not `.mdx`** — MDX parses Clojure's `{...}` in prose as JSX
  and fails the build.
- Starlight ≥0.39 sidebar autogenerate syntax: `items: [{autogenerate:…}]`.
- Astro content cache lives in BOTH `.astro/` and `node_modules/.astro/` —
  stale entries after rename/delete need both cleared.
- Comment-block prose quality varies: newer cljg-* tests read like docs;
  terse old tests need the manifest `lead` line to carry the teaching hook.
