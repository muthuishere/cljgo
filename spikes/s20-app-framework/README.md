# S20 — the application framework (working name: keel)

Owner mandate (2026-07-17): cljgo ships a batteries-included application
framework — the capability set of Spring Boot, the simplicity of
Rails/Elixir — **library style**: you call it, it never calls you. No
classpath scanning, no annotations-as-magic. Pillars: HTTP server ·
middleware · routing · configuration · data layer · worker queues ·
cache · AI providers. Simplicity is the core value; Clojure style, not
Go style.

This spike is design-with-prototypes (ADR 0027): most of the answer is
positions in VERDICT.md + ADR 0041 + an OpenSpec change, but the 2–3
riskiest UX claims are DEMONSTRATED, not asserted.

## The one question

Can cljgo's unfair advantages (live vars, real goroutines, require-go,
Result/Option, single binary) carry a Rails-grade golden path — a whole
small app in under a page of Clojure — without inverting control?

## Exit criteria (written before any code)

1. **Live-handler claim demonstrated**: an HTTP server whose handlers
   are resolved through cljgo VARS serves a request, the handler is
   re-`def`ed through the real evaluator, and the NEXT request answers
   differently — no restart, no reconnect. (Prototype embeds pkg/eval;
   today's interp seed registry has no net/http, so the adapter side is
   Go — exactly the adapter the framework would ship.)
2. **Routes-as-data claim demonstrated**: a plain Clojure data structure
   (vector of [method+pattern handler-var]) written in Clojure, walked by
   the adapter, mounted on Go 1.22+ `net/http.ServeMux` patterns —
   method matching and `{id}` path params work without a router engine.
3. **Config claim demonstrated**: an EDN config file read through the
   real cljgo reader, overlaid by `APP_`-prefixed env vars, producing
   one plain map — precedence measured (env > file > default).
4. **Worker claim demonstrated**: `enqueue` → goroutine worker executes
   a job, entirely in interpreted cljgo (`cljgo run`), with a
   persistence seam shown (every state transition journaled through one
   function that a Postgres backend can replace) — zero brokers.
5. VERDICT.md takes a position per pillar (ONE blessed way each),
   ADR 0041 is drafted (status: proposed), the OpenSpec change exists
   with tiered tasks, and the golden-path app is REAL CODE under one
   page.
6. Three adversarial DHH-persona review rounds are run by fresh
   subagents; each review is committed verbatim under `reviews/`, each
   forced substantive revision (recorded in VERDICT.md); unresolved
   round-3 objections are logged as open questions for the owner.

## Layout

- `prototype/` — standalone Go module (replace-directive on the repo
  root, s17-style) proving criteria 1–3 against the real pkg/eval,
  pkg/reader, pkg/lang. Throwaway; never merges into pkg/.
- `prototype/workers.clj` — criterion 4, pure interpreted cljgo.
- `run.sh` — drives everything, prints PASS/FAIL per criterion.
- `VERDICT.md` — the positions + measured evidence.
- `reviews/dhh-round-{1,2,3}.md` — the adversarial rounds, unsoftened.
