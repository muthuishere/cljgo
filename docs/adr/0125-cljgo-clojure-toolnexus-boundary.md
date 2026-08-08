# ADR 0125 — The cljgo / Clojure / toolnexus boundary

Date: 2026-08-08 · Status: **proposed** (owner rules; this document does not
self-approve). Extends the two-way test of ADR 0102 to three repos. Touches
ADR 0081 §3 and ADR 0075's battery catalog. **No code moves under this ADR.**

## Context

The owner asked (01-AUG) for a ruling on where things live: "what belongs in
cljgo stays in cljgo, what belongs in Clojure stays in Clojure, what belongs in
a toolnexus split stays there." The discussion has been long and produced no
path. The facts as verified today:

- **toolnexus** is not a Go library — it is a seven-language agent SDK (js,
  python, golang, java, csharp, elixir, clojure) with no root `go.mod`,
  published to npm/PyPI/NuGet/Maven/Hex/pkg.go.dev **and Clojars**.
- Its **Clojure split already targets cljgo**: one `.cljc` tree, zero reader
  conditionals, sole third-party dep `koine` — all three verified in the tree.
  Its README additionally reports a full suite green in 5 execution modes
  against the *published* cljgo v0.9.0 archive; that figure is **quoted from
  `toolnexus/clojure/README.md` (dated 2026-08-02), not re-run for this ADR**,
  and this repo's claims discipline forbids treating a README as a
  measurement. It is cited here as intent, not as evidence.
- `cljgo-gate.sh` exists in the toolnexus tree and its own header describes it
  as a downstream gate for cljgo CI. **As of this ADR, cljgo's CI referenced it
  nowhere** — the wiring is being added separately (`.github/workflows/
  downstream.yml`). Do not read the script's self-description as proof it was
  running.
- **cljgo imports nothing from toolnexus.** The dependency is one-way and clean.
- The only recorded boundary directive is a buried paragraph in
  `docs/adr/0081-bri-cli-agent-skills.md:58` — "LLM/tool orchestration uses
  toolnexus (owner directive, 2026-07-24)".

## Decision

### 1. The three-way test (one sentence, applicable without asking)

> It is **cljgo** if it is a mechanism the *language and its runtime* must
> provide (ADR 0102's `cljg.*`) or an opinion about *how you build an app*
> (`bri.*`); it is **Clojure** if `clojure.core` already defines it, in which
> case cljgo implements it and never renames or re-shapes it (ADR's precedence
> principle); it is **toolnexus** if the capability is *about agents and LLMs*
> and would have to exist identically in seven languages — because anything an
> agent SDK owns must be cross-port, and cljgo is one port, not the owner.

Corollary, and the tie-breaker in practice: **cljgo exposes itself to agents;
toolnexus lets a program drive agents.** Outward-facing self-description is
cljgo's; LLM/tool orchestration is toolnexus's.

### 2. Ownership table

| Disputed piece | Owner | Why (one line) |
|---|---|---|
| `--json` structured output, diag envelopes (ADR 0081 §1) | **cljgo** | It is the CLI's own I/O contract; nothing agent-specific about it. |
| `<app> --schema` (typed tool list from the ADR 0078 command tree) | **cljgo** | A projection of a declaration only cljgo holds; no other port can compute it. |
| `SKILL.md` generation for a bri.cli app | **cljgo** | Same projection, same source of truth — but the *file format* follows toolnexus `skill.cljc`, it does not invent a second one. |
| `<app> mcp` stdio server (ADR 0081 §3) | **toolnexus** | Serving MCP is protocol plumbing that must be identical in 7 languages; toolnexus already ships `golang/mcpserve.go` + `clojure/src/toolnexus/{mcp,serve}.cljc`. |
| `bri.ai` / `bri.llm` battery | **toolnexus** | LLM/tool orchestration — the 2026-07-24 owner directive, now ratified here instead of buried. |
| Agent/tool/a2a/translate/streaming APIs | **toolnexus** | Cross-port SDK surface; cljgo consumes it as a Clojars dep like any library. |
| `koine`, `clojure.core` semantics, reader | **Clojure/cljgo** | Precedence principle: cljgo implements Clojure, never a variant of it. |
| Cross-port example parity (Clojure vs Go/JS/Python) | **toolnexus** | See §5. |
| Clojure↔cljgo example parity | **toolnexus (done)** | Already machine-enforced there by symlinked `src` + `run-both.sh` byte-diff. |

### 3. Anything currently in the wrong place?

**No code is misplaced.** cljgo imports nothing from toolnexus; toolnexus's
Clojure split depends only on cljgo + koine and gates itself against a
published cljgo archive. The one-way arrow is already correct, and it is worth
saying plainly rather than inventing a migration.

What *is* misplaced is a **pending decision, not code**: ADR 0081 §3 proposes
building an MCP server inside bri.cli, and ADR 0081's prose reserves an eventual
`bri.ai`/`bri.llm` battery "under ADR 0075" — while the same paragraph names
toolnexus as the blessed path and `spikes/s47-native-tui-fundamentals/VERDICT.md:55`
records the capability as "bri.ai / toolnexus, deferred". Verified: `bri.ai` does
**not** appear in ADR 0075's catalog table, so the reservation exists only as
prose there.

But it is reserved in more places than that, and an earlier draft of this ADR
undercounted them — which matters, because a ruling that amends two documents
while four others keep the reservation alive would itself become a document
asserting something untrue. The full live set:

| where | line | what it says |
|---|---|---|
| `docs/adr/0041-app-framework.md` | 61 | `keel.ai` in the fixed namespace list |
| `docs/adr/0041-app-framework.md` | 169 | "AI (owner pillar; OUT of this change's scope)" |
| `docs/adr/0081-bri-cli-agent-skills.md` | 60 | "an eventual `bri.ai`/`bri.llm`" |
| `openspec/changes/app-framework/proposal.md` | 31, 74 | `bri.ai` OUT of scope; deferred satellite |
| `openspec/changes/app-framework/tasks.md` | 283 | "first-party, independently versioned satellite" |
| `spikes/s47-native-tui-fundamentals/VERDICT.md` | 55 | "bri.ai / toolnexus, deferred" |

Note ADR 0041 line 4 records the `keel` → `bri` rename explicitly, so `keel.ai`
**is** `bri.ai` and cannot be waved off as a dead naming scheme. `app-framework`
is an *active* (unarchived) openspec change, so it is item 4 of the authority
chain, not history. Seven mentions, one capability, no ruling.

### 4. Ruling on the two collisions — and why it is free now

Both are **unbuilt**. Verified: ADR 0081 is `status: proposed`; there is no MCP
implementation anywhere in `core/`, `pkg/`, `cmd/`, or `templates/` (the only
`mcp` hits are unrelated). toolnexus already ships the working implementations.
Ruling now costs nothing and prevents a second implementation later.

- **(a) `<app> mcp` → toolnexus.** bri.cli keeps `--json`, `--schema` and
  `SKILL.md` (projections of its own command tree, which nothing else can
  produce) and **drops the MCP server from ADR 0081 §3**. Serving MCP is
  protocol work with seven ports' worth of duplication ahead of it; toolnexus
  owns it and has it. A cljgo app becomes an MCP server by handing its
  `--schema` output to toolnexus — one implementation, still two front doors.
  ADR 0081 §3 should be amended (superseded in part), not silently ignored.
- **(b) `bri.ai` / `bri.llm` → toolnexus; do not reserve the namespace.** The
  battery is deleted from the roadmap rather than renamed. Naming an empty
  `bri.*` slot for a capability we have already ruled belongs elsewhere is how
  the same thing gets built twice. If a bri app needs an LLM, it requires
  toolnexus from Clojars — subject to the standard pure-Go / `CGO_ENABLED=0`
  gate every bri dependency passes.

Both rulings shrink cljgo's roadmap. That is the point: simplicity first
(owner, 2026-07-31) — declining to own a capability is cheaper than owning it.

### 5. Example parity — the premise was wrong

Clojure↔cljgo example parity **already exists** inside toolnexus, enforced at
the source level: the example `src` directories are **symlinks to one file**,
so the two hosts cannot drift by construction. There is nothing for cljgo to do
here.

Be exact about how much is *machine*-enforced on top of that, because the
tempting summary ("byte-enforced in CI") is wrong three ways: `run-both.sh:37`
defines `norm()` which `sed`s the runtime-name line before diffing, so it is a
normalised diff, not a byte diff; it compares **one** of the examples
(`toolnexus.demo`), while the other four are only asserted to print `OK` on
each host separately; and **no CI job runs it** — toolnexus's `ci.yml` runs
`all-modes-check.sh` and the two per-host `run.sh` scripts. The symlink is the
real guarantee; the script is a spot check.

The real gap is **cross-port**: the Clojure split ships ~5–6 examples against
9–13 in go/js/python (missing streaming, hooks, advanced, translator, a2a).
That is toolnexus-side work and belongs on toolnexus's backlog, not cljgo's.
Recording it here so it stops being re-discovered as a cljgo problem.

## Consequences

- A contributor can place any new capability with §1 alone.
- ADR 0081 §3 is superseded in part (MCP server out).
- **`bri.ai` is retired as a reservation, and retiring it means editing all six
  live sites, not two.** On ruling, each of the rows in the table above gets a
  one-line pointer to this ADR: ADR 0041 (:61, :169), ADR 0081 (:60), the
  active openspec change `app-framework` (`proposal.md:31,74`,
  `tasks.md:283`), and spike s47's VERDICT (frozen — annotate, do not edit;
  add the note to this ADR's own record instead). ADR 0075's catalog gains no
  `bri.ai` entry, which is a no-op there since it never had one. Leaving any of
  these would leave the boundary contradicted in the authority chain — the
  precise defect ADR 0124 was opened for, one level up.
- cljgo's agent story becomes: *self-describe* (`--json`, `--schema`,
  `SKILL.md`) and *depend on toolnexus* for everything that drives an agent.
- The dependency stays one-way. If cljgo ever needs to import toolnexus into
  its own toolchain (as opposed to a user's app doing so), that is a new ADR —
  it would make a downstream consumer an upstream dependency.
- Risk: `--schema`/`SKILL.md` formats could drift from toolnexus's. Mitigation
  is stated above — cljgo follows toolnexus's `skill.cljc` shape rather than
  defining its own.
