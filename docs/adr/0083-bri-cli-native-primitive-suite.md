# ADR 0083 — bri.cli is a suite of native, pure-Go, cross-platform CLI primitives (not a TUI framework)

Date: 2026-07-24 · Status: accepted (owner-directed). Reframes ADR 0078 (bri.cli)
and revises its §7 backend choice; builds on ADR 0074/0076 (opt-in linking), ADR
0077 (`cljgo dist`), ADR 0060 (pluggable secrets/vault), ADR 0080 (OpenAPI auth).
Backed by spikes s47 (native TUI fundamentals) + s48 (primitive feasibility).

## Context

ADR 0078 framed bri.cli around a rich interactive layer, and spike s46 recommended
the Charm stack for it. The owner **overrode** that: *"we don't need all Charm has,
we need PRIMITIVES … we've adapted everything natively across cljgo and never
depended on such a library — we can't take that risk on the core. Terminal UX is
cool, but fundamentals and separate libraries are the core."* The real ask is not
a TUI framework; it is a **suite of tight, native primitives** that solve the
genuinely-hard cross-platform CLI problems, owned by us so the core never bets on
a heavy dependency.

Two spikes settled feasibility:
- **s47** — a from-scratch minimal Elm-architecture TUI (loop + diff renderer +
  input events + select + editor widgets) is **+0.53 MB / ~5× smaller than
  Charm**, pure-Go, cross-compiles, and its portable half is plain Clojure (runs
  on cljgo AND the JVM). opencode's own TUI is Bubble Tea, so "opencode-class" is
  just owning this loop — which we can, minimally.
- **s48** — every primitive's backing is **pure-Go / `CGO_ENABLED=0` /
  cross-compiles** (incl. Windows SCM via `x/sys`), so cgo never forces a
  dependency; own-vs-wrap is a free choice per primitive.

## Decision

### 1. bri.cli is a suite of separate, opt-in, native primitive libraries

Not one monolith and not a TUI framework — a set of `bri.*` namespaces, each its
own hard-problem, each **opt-in linked** (ADR 0074/0076: links only when
required), each a clean Clojure surface over a **thin** platform primitive with
**minimal/no heavy deps**. The core owns its fundamentals; any third-party library
is an opt-in leaf, never the trunk. Where a primitive's portable half is plain
Clojure over a small platform shim, the primitive targets **both cljgo (static
binary) and JVM Clojure** — reach a Go-only dependency could never give.

### 2. The primitive set

| namespace | what it gives | native backing (own, or thin-wrap) |
|---|---|---|
| `bri.cli` | args + basic **colored** prompts (input/select/confirm/password) — the s47 native core; terminal UX is deliberately minimal | **own** (s47), no Charm |
| `bri.cron` | scheduling (cron expr → callbacks) | **own** (~100 LOC), or robfig/cron |
| `bri.service` | install/start/stop/status as a system service across **systemd · launchd · Windows SCM**, one API | **own** over `x/sys` (systemd unit + launchd plist are text-gen) |
| `bri.openapi` | give an OpenAPI spec → a typed client / generated CLI | **own** the subset a CLI needs (paths/params/ops/security) |
| `bri.secrets` | OS-keystore credential store (save/get) + login flows (web-page OAuth **or** API key), values in the secret store — never argv/logs/context | **own** the provider iface (ADR 0060 / s39: cgo-free), ties to bri.auth |

`publish` is already covered (`cljgo dist` 0077, `publish npm` 0079, adapters
0082). The catalog is append-only; each primitive lands on its own ADR → spec →
dual-mode → gates.

### 3. Terminal UX is minimal by intent

`bri.cli` provides color + basic prompts on the s47 native core — enough to be
pleasant, small, and portable. It is NOT a Charm-scale TUI, and that is the point.
A user who wants a rich TUI brings Charm (or anything) themselves via `go-require`
interop; the core does not depend on it and is not shaped by it.

### 4. Constraints (binding on every primitive)

Pure-Go / `CGO_ENABLED=0` (proven for all in s48); cross-compiles via `cljgo dist`;
opt-in with zero cost when unused; one blessed way per problem (precedence
principle — never shadow clojure.core); dual-mode parity (interpreted = compiled)
where the logic is deterministic; secrets are use-only (never in context/argv/logs
— the owner's standing rule); minimal dependency surface, own the fundamentals.

## Consequences

- bri.cli becomes what a one-person CLI author actually needs: parse+prompt, ship
  everywhere (`dist`/npm), schedule, install as a service, talk to an OpenAPI API
  with real auth, and keep credentials in the OS keystore — each a small native
  library, none dragging a heavy framework into the core.
- The Charm risk is gone: no big library in the trunk; the interactive core is
  ours, ~5× smaller, and portable to the JVM.
- The suite grows one ADR at a time (like the ADR 0075 battery catalog), gated on
  the pure-Go + cross-compile + opt-in constraints s48 proved achievable.
- Revises ADR 0078 §7: the interactive backend is the **native s47 core**, not
  Charm. ADR 0078's parameter model (validators, trim, the resolution pipeline)
  stands; its prompts are rendered by the native core. Supersedes spike s46's
  recommendation (a valid measurement under a different objective function).
- Build order (each its own ADR/spec): `bri.secrets` + `bri.openapi` feed ADR
  0080's authenticated OpenAPI CLI; `bri.service` + `bri.cron` are the "runs
  unattended" pair; the native prompt core completes ADR 0078 increment 2.

## Not chosen

- Charm (or cobra/viper/survey) as a **core** dependency — owner-rejected on
  size, performance, portability, and dependency risk. Opt-in, user-brought only.
- A single monolithic bri.cli — the primitives are separate opt-in libraries so an
  app links only what it uses.
