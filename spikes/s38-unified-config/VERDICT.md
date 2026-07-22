# s38 — VERDICT

**Verdict: GREEN (feasible, recommend adopting).** A single 6-layer resolver
composes cleanly on top of today's `bri.config`, in pure Go with `CGO_ENABLED=0`
and one pure-Go dep (`olympos.io/encoding/edn`). All five exit criteria pass in
a runnable demo. The existing 2-layer contract (`load!`, `explain`, per-key
provenance, deterministic `APP_*` mapping) is a strict subset — layers 1–4 are
today's behavior; layers 5–6 (vault stub, DB-primary) are new and additive.

## Evidence (`CGO_ENABLED=0 go run .`)

All demos print `PASS`. Build + `go vet` clean with CGO disabled.

| Criterion | Proof in demo | Result |
|---|---|---|
| 1. 6-layer precedence | one key tops out at EACH layer; asserted | PASS |
| 2. `.edn` == `.properties` map | both parse to identical `reflect.DeepEqual` map | PASS |
| 3. Provenance | resolved dump prints winning layer per key | PASS |
| 4. Runtime hot-reload | rotate row → next read = new value, no restart; 5 reads add 0 table reads (read-through cache); audit rows | PASS |
| 5. Coercion + masking | durations/sizes are `int64`; env `"8080"`→`int64`; secrets masked in every dump | PASS |

### How the 6-layer precedence resolved (the demo's key ladder)

```
app.name        <- default    (only in built-in defaults)
db.host         <- file       (application.edn)
log.level       <- profile    (application-prod.edn, APP_PROFILE=prod)
server.port     <- env        (APP_SERVER__PORT=8080, coerced to int64)
db.password     <- vault      (secret, masked)
db.pool-size    <- runtime    (DB row 50 beats default 5, file 10, profile 20, env 30)
openai.api-key  <- runtime    (secret via DB row, masked)
```

Cross-checked against the real `clojure` CLI: `clojure.edn/read-string` of
`equiv.edn` equals the exact nested map the Go resolver builds (kebab-case
keyword keys, `int64` numbers, nested maps) — `true`.

## The `.edn` vs `.properties` call — RECOMMEND: EDN canonical, `.properties` accepted

- **EDN is canonical.** It is Clojure-native (the reader, `clojure.edn`), the
  language stays first-class (the precedence principle), and it expresses maps,
  vectors, keywords, and tagged literals that `.properties` cannot.
- **`.properties` is an accepted convenience skin**, not an equal. It parses
  into the IDENTICAL canonical nested map (dotted key → nested path, values
  coerced). It exists only for Java-familiar teams and simple flat overrides.
  When both files exist for the same base name, EDN loads first and
  `.properties` overlays — but the blessed guidance is **one format per
  project**. This matches ADR 0041's "one blessed way per pillar" (alternatives
  possible, not documented as equals).

## Recommended BLESSED form

- Keep `(config/load!)` / `(config/explain)` byte-compatible; grow the resolver
  underneath from 2 → 6 layers. `load!` gains an optional opts map
  (`{:runtime store :vault v}`).
- Add `(config/get cfg path [default])` and `(config/source cfg path)` as the
  app-facing read + provenance API (see `config_sketch.cljg`).
- DB-primary layer = a `runtime_config`-shaped store (key, value, is_secret,
  updated_by, updated_at) behind a read-through cache with write-time
  invalidation, single mutation path `(config/rotate! store path v opts)`.
  In-memory stand-in here; real backing is pgx (bri.db, T2).
- Vault as a Clojure protocol (`lookup`, `secret-paths`) so the real secrets
  backend binds later without touching apps.
- Secrets masked everywhere (`ab…yz (secret, masked)`); AES-GCM at rest is a
  prod concern of the DB layer, out of the resolver core.

## UN-PROVEN risks / out of scope

- **No real DB.** The DB-primary layer is an in-memory stand-in; pgx wiring,
  AES-GCM at rest, and the direct-SQL-staleness gotcha (kernel #1) are modeled
  but not implemented. Real Postgres round-trip latency on cache-miss is
  unmeasured.
- **No schema enforcement across new layers.** Today's `conf.schema.edn`
  validate! was not extended to the 6-layer map; needs a pass so a
  vault/runtime value can't violate the schema silently.
- **Vault is a stub** — no real backend, no rotation-of-the-vault-key path.
- **`.properties` type coercion is heuristic** (int → float → bool → string).
  EDN carries explicit types; `.properties` guesses. A value like `08` or a
  version string `1.20` could surprise. EDN-canonical sidesteps this.
- **Env→path mapping ambiguity** unchanged from today: `_` join vs `__` split
  means a segment can't itself contain `__`. Pre-existing, not worsened.
- Resolver rebuilds the full map per `Resolve` call; fine for boot + rotate,
  not tuned for per-request hot reads (the DB layer's cache is, though).

## Owner-gated questions

1. **Is `.edn` canonical + `.properties` accepted-skin the right call?** (Recommended.)
   Or EDN-only, and drop `.properties` to avoid the coercion-guess surface?
2. **Is the DB-primary runtime layer in scope for v1, or a later tier?** It
   pulls in bri.db (T2) as a dependency and an admin write path. Options:
   (a) ship layers 1–5 now (files + profile + env + vault), DB layer next;
   (b) ship all 6 with an in-memory runtime store first, wire pgx later.
3. **Vault in v1 or defer?** The stub interface costs nothing to reserve; a real
   backend (vsync / KMS) is a separate spike.
4. **Should `profile` become a file family (`application-{profile}`) as modeled
   here, replacing today's in-file `:profiles` section — or support both?**
   This spike adds the file family; ADR 0041 §4 froze profiles as an in-file
   `:profiles` map. They can coexist (in-file `:profiles` still merges), but the
   blessed guidance needs one recommended shape.
5. **Where do secrets live at rest** — DB (AES-GCM, reqsume model) only, or also
   a vault backend? Affects whether layers 5 and 6 overlap in responsibility.
