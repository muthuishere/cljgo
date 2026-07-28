# ADR 0104 — Docs as product: the cljgo Book, cljgo by Example, and per-package doc packs

Date: 2026-07-28 · Status: **proposed** (owner-directed: *"documentation should
look like rust book / rust by example … teach clojure and cljgo for any new
guy … every package a separate documentation pack … which to use when …
marketing is core"*).

## Context — measured, not guessed (docs inventory 2026-07-28)

The site (Astro+Starlight, 23 pages) is good marketing + bri guides, but:

- **6 of 30 registered namespaces are documented; 21 have ZERO docs** — the
  entire `cljg.*` mechanism tier (io, os, net.http, http, socket, net.dns,
  compress, security, secrets, cache, jobs, stream, process, system, date) and
  the whole `bri.cli` family, despite ADRs 0083–0103 specifying each.
- **Nothing teaches Clojure itself.** Every page assumes a Clojure programmer;
  a "new guy" has no path from zero.
- **No which-to-use-when.** Three tiers (`clojure.*`/`cljg.*`/`bri.*`, ADR
  0085) with no page telling a user which to reach for.
- **Huge untapped raw material:** 521 runnable conformance examples with
  frozen `;; expect:` output (CI-tested by construction), 96 ADRs, 27
  diagnostic explain pages, 8 runnable examples/, rich git commit narratives.
- **No generation tooling; hand-duplicated content drifts** (docs/guides vs
  site/bri mirror by hand; one site page still says `bri.core.security`).

Rust's docs are the industry benchmark because of five properties: one linear
**Book** that teaches the language from zero with a running project; **Rust by
Example** as a parallel runnable-example track; a **reference for every
module**; explicit **which-to-use-when** pages; and **doctests** so examples
cannot rot. cljgo already owns the hardest part — a corpus of frozen, CI-run
examples — and doesn't show it.

## Decision

Documentation becomes a **product surface with the same discipline as code** —
four pillars, one repo, one site, all example-first, all CI-verified:

1. **The cljgo Book** (`site/.../book/`) — a linear course that teaches
   **Clojure itself AND cljgo together**, Rust-Book-style: values → fns →
   collections → destructuring → flow → state (atoms) → lazy seqs → protocols
   → macros → errors → concurrency (core.async) → interop → build&ship. Every
   chapter drives a running project forward (a real CLI+web tool built with
   the batteries), every snippet runnable, zero prior Clojure assumed. The
   final chapters are cljgo-specific (REPL workflow, AOT, deploy, taxonomy).
   **Per-language on-ramps (owner mandate 2026-07-28): a separate
   fundamentals-first track for each arrival language — "Coming from Go",
   "Coming from Java", "Coming from Python", "Coming from C"** — each a short
   bridge (their concept → the Clojure/cljgo way, side-by-side code: structs→
   maps, classes→data+fns, loops→seq ops, goroutines→core.async, exceptions→
   ex-info/Result, packaging→build.cljgo) that lands the reader at the right
   Book chapter. These are also acquisition pages (SEO: "clojure for go
   developers").
2. **cljgo by Example** (`site/.../by-example/`) — the EXACT mdBook shape of
   doc.rust-lang.org/rust-by-example (owner-shown reference): ONE numbered
   linear course in the sidebar (*1. Hello World · 2. Values · 3.
   Collections · … · 14. Pattern matching → 14.1 / 14.2 nested sub-pages*),
   prev/next arrows through the whole sequence, and every page = short prose
   + annotated runnable code + its real output. Concepts first, then the
   battery topics continue the same numbering (…*18. Serve HTTP · 19.
   Sockets · 20. Passwords & keychain*…). **Generated where possible from
   `conformance/tests/`** — the 521 frozen examples are by-example pages CI
   already guarantees; a generator maps file → page (title from filename,
   code block, `;; expect:` as shown output), hand-written pages only where
   no conformance file fits (and each hand-written example gets a
   conformance twin so it can't rot — the doctest rule).
3. **Per-package doc packs, surfaced under HUMAN NAMES (owner mandate:
   "packages buried by better names")** — the navigation NEVER shows a raw
   namespace string. Nav labels are task-named topics — *"Serve HTTP"*,
   *"Sockets"*, *"Passwords, crypto & keychain"*, *"Caching"*, *"Background
   jobs"*, *"Talk to a database"*, *"Compression"*, *"DNS"* — and the
   namespace (`cljg.security`, …) appears INSIDE the page as the require
   line, a detail not a label. EVERY `Specs()` row and every clojure.*
   satellite maps into exactly one pack: `index` (what/why/when — including
   **"use this when / use X instead when"** cross-links across the three
   tiers), `api` (every public var: signature, one-line, example), `howto`
   (task-oriented recipes), `examples` (links into by-example). Source
   material: the namespace's ADR(s), docstrings, conformance files, and git
   history. **A CI check fails when a Specs() row has no pack** — coverage
   is gated like conformance, so wave N+1 can never ship undocumented again.
4. **Which-to-use-when as a first-class page family** — the taxonomy page
   (clojure vs cljg vs bri with the ADR 0085/0103 line), plus per-domain
   deciders (http client vs server vs bri.web; cljg.secrets vs
   cljg.security keychain; cache vs jobs; test tiers).

**Marketing is core:** the landing + why pages get the receipts surfaced —
"30 batteries, every one documented, every example CI-run", the honest
benchmark story, and the Book as the acquisition funnel ("learn Clojure on
cljgo" targets newcomers, not just Clojure veterans — the biggest untapped
audience).

### Mechanism (how it stays true)

- **`cmd/gendocs`** (or a site-build script): conformance→by-example page
  generation, docstring→api-page extraction, pack-coverage check. Same
  spirit as genbri: generated pages are never hand-edited.
- **One source of truth:** site content lives in `site/src/content/docs/`
  only; `docs/guides/*` become pointers (kills the hand-mirroring drift).
- **Examples-run-in-CI rule:** any fenced snippet marked runnable must have a
  conformance twin or be generated from one. Prose can't claim what CI
  doesn't run.
- Git history and ADRs are the writer's quarry (the "expanded with git
  comments" mandate): each pack's index cites its ADRs; changelog-style
  "what landed" notes are mined from commit messages.

## Consequences

- Docs debt becomes visible and gated (pack-coverage CI) instead of silent.
- The Book is the single biggest new artifact (~15–20 chapters) — staged
  delivery; by-example + packs are heavily generated so they scale with the
  30-namespace surface at bounded cost.
- New-namespace definition-of-done grows: code + conformance + **doc pack**.
- The site sidebar reorganizes around the four pillars (Learn / By Example /
  Packages / Guides+Reference); existing 23 pages are re-homed, not deleted.

## Process

1. Wave D1: tooling (gendocs conformance→by-example + coverage gate) +
   taxonomy/which-to-use-when pages + packs for the 21 uncovered namespaces
   (agent-team-friendly: one pack per agent, ADR+source+conformance as input).
2. Wave D2: the Book, chapters in order, each chapter PR-sized.
3. Wave D3: marketing refresh on top (landing receipts, learn-clojure funnel).
