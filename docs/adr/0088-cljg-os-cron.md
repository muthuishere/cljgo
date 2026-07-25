# ADR 0088 — `cljg.os` (scheduler first): a cgo-free cron scheduler; service management follows

Date: 2026-07-25 · Status: accepted (owner: *"the core must solve … running a
scheduler and creating a system service across OS"* — spike s48). Second `cljg.*`
stdlib namespace (after ADR 0087's `cljg.net.http`); realizes the `cljg.os` tier of
ADR 0085.

## Context

A cljgo CLI often needs to *run unattended*: schedule periodic work, and install
itself as an OS service. ADR 0085 placed both under **`cljg.os`** (env · signals ·
service · cron · …). s48 proved each is pure-Go / `CGO_ENABLED=0` / cross-compile,
so the choice is never forced by cgo.

The two split cleanly by testability: **cron** (parse an expression, compute the
next fire, tick) is pure, deterministic logic — fully unit-testable. **Service**
(install/start/stop across systemd / launchd / Windows SCM) is thin text-generation
plus shelling out to an init system that CI cannot exercise. So `cljg.os` lands in
two increments; this ADR ships the scheduler and reserves the service surface.

## Decision

### 1. `cljg.os` — cron scheduler (this increment)

A standard 5-field cron (`min hour day-of-month month day-of-week`, with `*`, lists
`a,b`, ranges `a-b`, and steps `*/n` / `a-b/n`; Vixie day-OR-day semantics when both
day fields are restricted). The **parser + next-fire** is a Go shim (`-cron-next`,
robust date math over stdlib `time`, minute-stepping search) — deterministic and
directly unit-tested. The **scheduler loop** is portable Clojure over three host
primitives (`-cron-next`, `-now-millis`, `-sleep-millis`): find the soonest due job,
sleep to it, run its fn, repeat. Job fns are Clojure — no Go→Clojure timer callback.

Surface:
```clojure
(require '[cljg.os :as os])
(os/cron-next "*/5 * * * *" (os/now))   ; next fire (epoch ms) — pure, testable
(def job (os/job "0 9 * * 1-5" #(sync!)))   ; weekday 09:00
(os/run [job …])                            ; blocking scheduler loop (a daemon's main)
(os/run [job …] {:max-ticks 3})             ; bounded — for tests / one-shots
```
`-cron-next` is host (Go `time`), so on the JVM host it is a JLine-style small
reimplementation; the loop, job model, and expression semantics are portable.

### 2. Non-OptIn, pure-Go

`time` is stdlib — no dependency to isolate, so `cljg.os` is a normal namespace, and
`CGO_ENABLED=0` + `cljgo dist` hold. It rides the same name-generic embedded registry
(ADR 0087 §1).

### 3. Service management — reserved (next increment)

`cljg.os` will add `service-install` / `service-start` / `service-stop` /
`service-status` over systemd units + launchd plists (generated text) and Windows SCM
(`x/sys/windows/svc`) — s48/s49-proven cgo-free. The generated unit/plist text is
unit-testable; the install/start/stop shell-outs are covered by build + a documented
manual check (CI has no init system). Deferred here to keep this increment focused
and fully testable.

## Consequences

- cljgo gains an in-process scheduler — a daemon can run cron jobs — with a
  deterministic, tested next-fire core.
- `cljg.os` opens as the env/scheduler/service tier; service management is additive
  on the same namespace next.
- Pure-Go keeps the static binary + `cljgo dist`; the scheduler loop stays portable
  Clojure (host primitives are just time/sleep).

## Not chosen

- **`robfig/cron`** (a dependency) — the parser + minute-stepping search is ~150 LOC
  of Go and keeps the tree dependency-free (s48).
- **A Go-goroutine scheduler calling Clojure job fns** — Go→interpreter/compiled-fn
  callbacks are awkward; a Clojure-driven blocking loop over `-sleep-millis` keeps
  job fns in Clojure and the loop portable + testable (bounded `:max-ticks`).
- **Service in this increment** — its shell-outs aren't CI-testable; split out so the
  scheduler ships fully green now.
