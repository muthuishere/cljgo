# ADR 0124 — the error funnel spans `cljg.*` and `bri.*`, and the mapping lives in bri

- Status: accepted
- Date: 2026-08-08
- Relates to: ADR 0041 (the bri error funnel), ADR 0102 (`bri` core
  fundamentals moved to `cljg.*`), issue #206

## Context

`bri.web.http/recover` is the error funnel: it catches whatever a handler
throws and turns an ex-info tag into an HTTP status through
`default-error-map`. The shipped table read:

```clojure
{:http/bad-param 400 :cast/invalid 422 :db/not-found 404 :db/constraint 409 :else 500}
```

and `recover` dispatched on `(:bri/error (ex-data t))` alone.

ADR 0102 moved the data/DB mechanism out of `bri.*` into `cljg.data.cast`.
When it moved, it took its own error vocabulary with it, correctly namespaced
to itself:

- `one!` throws `{:cljg.data.cast/error :cljg.data.cast/not-found}`
  (`core/cljg/data_cast.cljg:87`)
- `cast!` throws `{:cljg.data.cast/error :cljg.data.cast/cast}` (:169)
- `connect` throws `{:cljg.data.cast/error :cljg.data.cast/no-url}` (:52)

Neither the key nor the values matched anything `recover` looked at.

Be precise about the consequence, because the obvious overstatement is
wrong: the abstract rows were **not** unreachable in general. Application
code that threw `{:bri/error :db/not-found}` by hand always got its 404,
and still does. What was unreachable was **every error the `cljg.data.cast`
layer itself raises** — which is the path the docs told users to rely on.
So `db/one!` on a missing row — the single most common "row is gone" path
in a bri app — returned **500** instead of 404, while a hand-thrown tag
worked. Measured before the fix, the four-case probe
`[plain-ex, one!, cast!, hand-thrown :bri/error]` returned
`500 500 500 404`: the last column is precisely the evidence that the
table itself was live and only the `cljg.*` throws missed it.

Three published places promised the opposite behaviour and had done since the
move: `docs/guides/bri-db.md:37`, `site/src/content/docs/bri/db.md:45` and
`:121` ("throws `:cljg.data.cast/not-found` … which the error funnel maps
straight to a 404"), plus the `one!` docstring itself ("the ADR 0041 funnel →
404"). The documentation was not wrong about the intent; the code had simply
stopped implementing it.

## Decision

**The funnel understands both tag keys, and the translation lives in `bri`.**

1. `recover` reads `(or (:bri/error data) (:cljg.data.cast/error data))`.
   `:bri/error` still wins when both are present, so `http/param!`,
   `bri.cli/*` and `config/*` behave byte-identically to before.
2. `default-error-map` gains rows for the concrete `cljg.data.cast` keywords
   next to the abstract ones it already carried:
   `:cljg.data.cast/cast → 422`, `:cljg.data.cast/not-found → 404`,
   `:cljg.data.cast/no-url → 500`.
3. `cljg.data.cast` is **not** changed. It does not learn `:bri/error`, does
   not require anything from `bri.*`, and keeps throwing its own namespaced
   keyword.

Point 3 is the whole decision. ADR 0102's taxonomy is that `cljg.*` is
**mechanism** — usable with no web framework in sight — and `bri.*` is the
opinionated **policy** layer built on top. "A missing row is a 404" is an HTTP
policy statement; it is meaningless to a CLI or a batch job calling `one!`.
So the dependency points the only direction it can: the layer that knows about
HTTP knows about the layer below it, never the reverse.

The alternative — having `cljg.data.cast` throw `:bri/error :db/not-found` —
would have been a one-word change and is the wrong one: it makes the
mechanism layer name a framework it must not depend on, and it re-couples
exactly what ADR 0102 separated.

We also chose a plain table over a translation function or a protocol. The
whole mapping is four keywords in the same `def` a user already overrides with
`(recover {:error-map {…})`. No second code path, no registry, no indirection
— the mechanism that already existed simply covers two vocabularies.

### What is deliberately not mapped

**There is no constraint-violation keyword in `cljg.data.cast`.** A UNIQUE or
foreign-key violation surfaces as the driver's own error through `-db-exec`,
carrying no `:cljg.data.cast/error` tag at all, so it falls to `:else` → 500.
The `:db/constraint → 409` row is therefore kept but fires **only when
application code throws it deliberately**. Inventing
`:cljg.data.cast/constraint → 409` here would have been a mapping that can
never trigger; classifying driver errors into a portable constraint keyword is
real work (per-driver SQLSTATE / error-code parsing) and belongs to its own
change.

## Consequences

- `(db/one! …)` in a handler is now a **404** and `(db/cast! …)` a **422**,
  with no handler code — the behaviour every doc already described. Statuses
  before → after for the probe: `500 500 500 404` → `500 404 422 404`.
- **This changes response status codes for existing apps.** An app that has
  been returning 500 on a missing row now returns 404. That is the documented
  and intended contract, but it is a visible behaviour change; an app relying
  on the 500 can restore it with `(recover {:error-map {:cljg.data.cast/not-found 500}})`.
- The JSON body's `:error` field now names the concrete kind, e.g.
  `"cljg.data.cast/not-found"`. It already named the kind; the kind is simply
  the fully-qualified one for these paths.
- Frozen by `conformance/tests/bri-http-recover-error-funnel.clj`, which
  includes a plain untagged `ex-info` → 500 **control** so a future reader can
  see the funnel is engaged and returning its own status, rather than reading
  a route-miss 404 as success.
- Every future `cljg.*` namespace that throws a tagged ex-info and wants an
  HTTP status must add its row to `default-error-map` **in bri**. That is a
  known, one-line maintenance cost, and it is the price of the layers staying
  the right way round.
- The three doc sites and the `one!` docstring needed no edit: they described
  this behaviour already, and now they are true.
