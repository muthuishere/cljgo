---
title: "bri.config"
description: "Configuration in bri: one plain EDN map, a :profiles section, a deterministic APP_* env overlay, and an optional schema that is enforced when present."
---

Two layers into one plain map: `conf.edn` → `APP_*` env. No config classes, no watchers, no remote sources.

```clojure
(def cfg (config/load!))   ; reads a file (and the env), no more
(:port cfg)                ; a plain map — get/get-in like any other
```

This is the `cfg` you saw on the golden page in the [tutorial](/cljgo/bri/tutorial/): a top-level def that constructs a value, with no other I/O.

## conf.edn and profiles

Profiles are a `:profiles` **section**, not a file family:

```clojure
{:port 3000
 :db   {:host "localhost" :pool-size 5}
 :profiles
 {:dev  {}
  :test {:db {:host "test-db"}}}}
```

`APP_PROFILE` selects the overlay (default `dev`; `cljgo dev` sets `dev`, `cljgo test` sets `test`). The selected profile deep-merges over the base map.

## Environment overlay

`APP_*` variables win over the file. The mapping is deterministic: `__` (double underscore) separates path segments, single `_` joins words —

```
APP_PORT=8080            → [:port]
APP_DB__POOL_SIZE=9      → [:db :pool-size]
```

Values coerce: all-digit → integer, `true`/`false` → boolean, else string. Durations and sizes are **numbers** (seconds, bytes) — no stringly-typed `"5m"`.

:::caution[Secrets are env-only]
`APP_SESSION_KEY` and friends never belong in `conf.edn`. (`APP_PROFILE` and `APP_SESSION_KEY` are selectors/secrets, not config data — they don't appear in the map.)
:::

## The schema (optional, enforced when present)

`conf.schema.edn` — generated minimal by `cljgo new --template web`, delete it to go schemaless:

```clojure
{[:port] {:type :int :required true :default 3000}}
```

- `:default` is the **lowest** layer (file, profile, and env all beat it).
- `:required` missing from every layer → `load!` **throws** before any server/pool/worker starts, naming the key. Misconfigured deploys must not boot.
- `:type` (`:int :number :string :boolean :keyword`) mismatches name the key **and** the layer that supplied the bad value.

## 2 a.m. debugging

```
$ cljgo config
profile: dev
  [:db :host]       "localhost"  <- file
  [:db :pool-size]  9            <- env
  [:port]           3000         <- file
```

Every key, its effective value, and the layer that won (default < file < profile < env).

## Where next

- [Tutorial](/cljgo/bri/tutorial/) — the full 15-minute walkthrough where `config/load!` fits
- [bri.http](/cljgo/bri/http/) — the server that consumes `(:port cfg)` and `APP_SESSION_KEY`
- [bri.html](/cljgo/bri/html/) — pages and forms
