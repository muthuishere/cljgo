# s38 — unified layered configuration + DB-primary runtime config

Spike (ADR 0027 lifecycle). Self-contained (`module cljgospike/s38`). Does NOT
touch `pkg/`, `core/`, `cmd/`, `conformance/`, or the root `go.mod`.

## Why

The existing `bri.config` battery (`core/bri/config.cljg` + `pkg/bri` shims) is
**two layers only**: `conf.edn` (+ `:profiles` overlay by `APP_PROFILE`) →
`APP_*` env, into one plain map, with per-key provenance and an optional
schema. ADR 0041 §4 froze it there deliberately.

The owner wants Java / Spring-Boot config ergonomics on top of that, AND the
**DB-primary runtime config** convention proven in production on Reqsume
(`reqsume-kernel/04-runtime-config-and-notifications.md`): a `runtime_config`
Postgres row WINS over files/env, is read through an in-memory cache, can be
**hot-rotated without a redeploy**, carries `updated_by` / `updated_at` audit,
and stores secrets AES-GCM at rest. Files/env become bootstrap-only seed.

This spike proves whether the two ideas compose into ONE resolver with a
clean 6-layer precedence, without breaking the existing `bri.config` contract.

## Exit criteria (answer each with runnable evidence)

1. **6-layer resolver, precedence proven.** Compose, low→high:
   `built-in defaults` → `application.{edn,properties}` →
   `application-{profile}.{edn,properties}` (profile from `APP_PROFILE`) →
   `APP_*` env (`__` splits path segments, `_` joins words:
   `APP_DB__POOL_SIZE` → `[:db :pool-size]`) → `vault` (stub interface) →
   `DB-primary runtime override`. Demo shows one key overridden at EACH layer
   and the highest present layer winning.
2. **`.edn` AND `.properties` parse to the SAME nested map.** Dotted
   `db.pool-size=10` → `{:db {:pool-size 10}}`. Show both files producing an
   identical resolved map. Decide: which is canonical?
3. **Provenance.** For every resolved key, report which layer won it (backs a
   future `cljgo config`). Print the resolved map with per-key source.
4. **Runtime hot-reload.** Change the DB-primary override at runtime; the next
   read reflects it with NO restart. Read-through cache + explicit
   invalidation; model `updated_by` / `updated_at` audit. In-memory stand-in
   for the table is fine.
5. **Type coercion + secret masking.** Durations / sizes resolve as NUMBERS,
   not `"5m"` strings; secret keys are never printed — masked in the
   provenance dump.

Plus: a `.cljg` sketch of the app-facing API (`(config/get cfg [:db :pool-size])`,
`(config/source cfg [:db :pool-size])`, runtime rotate) — the blessed shape.

## Constraints honored

- Pure-Go, `CGO_ENABLED=0`. Only dep: `olympos.io/encoding/edn` (pure Go).
- Batteries stay under `bri.*`; nothing shadows `clojure.core`.
- Extends the existing 2-layer battery — does not replace it. Layers 1–4 are a
  superset of today's behavior (defaults, file, profile, env); layers 5–6 are
  new (vault stub, DB-primary).

## Layout

- `types.go` — canonical nested map, leaf paths, deep-merge, coercion, masking.
- `edn.go` — EDN file → canonical map (via `olympos.io/encoding/edn`).
- `properties.go` — `.properties` (dotted keys) → the SAME canonical map.
- `resolver.go` — the 6-layer composition + provenance.
- `runtime.go` — in-memory DB-primary layer: read-through cache, invalidation,
  audit fields, secret registry.
- `main.go` — runnable demo that PROVES criteria 1–5.
- `config_sketch.cljg` — app-facing `bri.config` API sketch (not wired).
- `testdata/` — `application.edn`, `application.properties`,
  `application-prod.edn`.

## Run

```
CGO_ENABLED=0 go run .
```

Verdict + recommendation in `VERDICT.md`.
