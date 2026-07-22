# ADR 0059 — Unified layered configuration + DB-primary runtime config
Date: 2026-07-23 · Status: accepted (owner-directed, 2026-07-23; evidence: spike
S38) · **Extends** the two-layer config of ADR 0041 §4 (Config) — additive, no
breakage. Consumes ADR 0060 (vault) as one layer.

## Context

ADR 0041's config is two layers: `conf.edn` → `APP_*` env. The owner wants
Java/Spring-Boot ergonomics — `application.properties` + environment + profiles
+ a pluggable vault — **and** the owner's own real convention (reqsume-kernel
ch04): **DB-primary runtime config** — rotate a provider key or a behavioural
knob *without a redeploy*, the DB row wins over env, a read-through cache, an
`updated_by`/`updated_at` audit trail, with `.env`/files as **bootstrap seed
only**. Spike S38 built a 6-layer resolver on top of the existing battery and
proved all of it.

## Decision — one resolver, six layers, ascending precedence

1. built-in defaults
2. `application.{edn,properties}`
3. `application-{profile}.{edn,properties}` (profile from `APP_PROFILE`)
4. `APP_*` env — deterministic mapping: `__` separates path segments, `_` joins
   words (`APP_DB__POOL_SIZE` → `[:db :pool-size]`)
5. **vault layer** (ADR 0060 provider, a `Get`-shaped hook)
6. **DB-primary runtime overrides** — row wins; read-through cache; audit
   (`updated_by`/`updated_at`); secrets AES-GCM at rest

Layers 1–4 are a **strict superset** of today's battery (`load!`/`explain`,
provenance, and the `APP_*` mapping are unchanged) — additive, nothing breaks.
**`conf.edn`/`conf.schema.edn` (the names ADR 0041 §2 scaffolds) stay accepted
as aliases**; `application.{edn,properties}` is the new blessed name `cljgo new`
emits going forward. Existing apps keep resolving with no change — that is what
makes layers 1–4 a true superset, not a rename.

- **EDN is canonical; `.properties` is an accepted Java-familiar skin** that
  normalizes into the *identical* nested map (dotted keys → nested; heuristic
  int→float→bool→string coercion where EDN carries explicit types). Blessed
  guidance: **one format per project**. S38 cross-checked the EDN parse against
  the real `clojure` CLI — byte-identical map.
- **Profiles:** the **file family** `application-{profile}.{edn,properties}` is
  the going-forward mechanism; ADR 0041 §4's in-file `:profiles` section stays
  honored for back-compat, file-family most-specific-wins. (Refines, does not
  reverse, 0041.)
- **Provenance:** every resolved key reports its winning layer — backs `cljgo
  config`. Secrets masked in every dump.
- **App API:** `(config/get cfg path [default])`, `(config/source cfg path)`,
  and `(config/rotate! store path v {:secret … :by …})` for the runtime layer.
  `load!`/`explain` stay byte-compatible.

**Scope:** layers 1–4 ship first (pure superset); layer 5 ships with ADR 0060;
layer 6 (DB-primary runtime) rides `bri.db` (0057/0058) — ship with the
in-memory store now, wire the real table when `bri.db` lands.

**Supersedes ADR 0041 §4's "secrets env-only":** layer 5 (vault, ADR 0060) means
a secret may now come from the OS keychain, an `age` file, or a cloud backend —
not only env. Env stays the bootstrap floor; the value never enters a config
dump (masked provenance) regardless of layer.

## Consequences

- cljgo config reaches Spring-Boot ergonomics while staying **EDN-first and
  single-binary**.
- The DB-primary runtime layer couples config to `bri.db` + an admin write path
  (the ch04 pattern) — the highest-leverage layer and the last to land.
- Un-proven (S38): real Postgres table, `validate!` schema across the new
  layers, AES-GCM at rest — modeled in-memory, not built.
- `config/get`/`config/source` reuse core names only as namespace-qualified
  vars (the `clojure.string/replace` convention, ADR 0056 filter #3) — the
  unqualified `clojure.core/get` is untouched.
- Not chosen: `.properties` canonical (loses Clojure types); env-only (defeats
  rotate-without-redeploy); an external config server / etcd (single-binary
  ethos).

**Constraint-filter #4 commitment (ADR 0056):** the resolver + `.properties`
skin + provenance land with dual-harness conformance (identical resolved map and
per-key source, interpreted AND AOT-compiled) and a boot-time budget (ADR 0024)
so layered resolution never regresses the boot floor.
