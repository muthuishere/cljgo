# Spike s48 — VERDICT: the bri.cli primitive suite is pure-Go + cross-compilable

Date: 2026-07-24 · Owner reframe: *"we don't need all Charm has, we need
PRIMITIVES … we cannot take the risk of depending on a [heavy] library … the core
must solve publish, getting inputs, running scheduler, creating system service
across OS, OpenAPI-as-builder, and secrets/keystore+login. Terminal UX is nice;
fundamentals + separate libraries are the core."*

This spike answers the one question that gates all of it: can each primitive be
done **pure-Go / `CGO_ENABLED=0` / cross-compile** (the sacred constraint, ADR
0023/0077) — so the own-vs-wrap choice is never forced by cgo?

## Result: yes — every primitive's backing is pure-Go and cross-compiles

`probe.sh` links the candidate backings for all cross-platform primitives and
builds them together:

- **host `CGO_ENABLED=0` build: OK** (5.3 MB combined), **0 cgo** in the closure.
- **cross-compiles all 5** ADR-0077 targets, **including Windows** (the system-
  service backing pulls the Windows SCM API — `x/sys/windows/svc` — cleanly under
  `GOOS=windows` build tags).

| Primitive | Candidate pure-Go backing | Own or wrap? |
|---|---|---|
| **inputs / prompts** | our own native TUI core (spike s47, +0.53 MB) | **OWN** (owner: no Charm) |
| **scheduler** (`bri.cron`) | a cron parser + ticker (~100 LOC), or `robfig/cron` | **OWN** (small) |
| **system service** (`bri.service`) | systemd unit + launchd plist = text-gen; Windows SCM = `golang.org/x/sys` (quasi-stdlib) | **OWN** over x/sys — no third-party |
| **OpenAPI builder** (`bri.openapi`) | own minimal parser over the subset a CLI needs (paths/params/ops/security), or `libopenapi` | **OWN a subset** (full spec is huge) |
| **secrets / keystore + login** (`bri.secrets`) | shell-out to OS tools (`security`/`cmdkey`/`secret-tool`) + `age` file fallback (spike s39 / ADR 0060), or `go-keyring` | **OWN** the provider iface (s39 proved cgo-free) |

## The architecture this enables

bri.cli is **not a TUI framework** — it is a suite of tight, native, **separate
opt-in libraries** (ADR 0074/0076 linking: each links only when required), one per
CLI hard-problem, each with a clean Clojure surface and **minimal/no heavy deps**:

- `bri.cli` — args + basic **colored** prompts (the s47 native core; terminal UX
  is intentionally minimal — color + prompts, not a Charm-scale TUI).
- `bri.cron` — scheduling.
- `bri.service` — install / start / stop / status as a system service across
  systemd · launchd · Windows SCM, from one API.
- `bri.openapi` — give an OpenAPI spec, get a typed client / a generated CLI
  (feeds ADR 0080's OpenAPI-default auth client).
- `bri.secrets` — an OS-keystore-backed credential store (save/get) + login flows
  (browser/web-page OAuth **or** API key), values kept in the secret store, never
  in argv/logs/context — ties to bri.auth (ADR 0069) + the pluggable-vault iface
  (ADR 0060).

Each is portable-Clojure logic over a thin platform primitive, so — like the s47
TUI core — a bri.cli app can target **both cljgo (static binary) and JVM Clojure**.
Power users who want Charm bring it themselves via `go-require` interop; the core
never depends on it.

## Not chosen

- A heavy TUI/CLI mega-framework (Charm, cobra+viper+survey stack) as a **core**
  dependency — the owner's explicit constraint. We own the fundamentals; libraries
  are opt-in leaves, never the trunk.
