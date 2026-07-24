# ADR 0082 — Publishing is a pluggable adapter surface in build.cljgo (override built-ins, register custom targets)

Date: 2026-07-24 · Status: proposed (roadmap; owner-directed: *"our build.cljgo
can have override for all this npm publish if they want to do something, also
adapters custom publish some other place and also something like that"*). Depends
on ADR 0021 (`build.cljgo` is a program), ADR 0054 (`publish go`), ADR 0077
(`cljgo dist`), ADR 0079 (`publish npm`). Fifth of the bri.cli block; generalizes
its distribution story.

## Context

cljgo now has three distribution mechanisms — `publish go` (ADR 0054), `dist`
(ADR 0077), and `publish npm` (ADR 0079) — each a fixed, built-in command. But
real projects need to (a) **override** a built-in's behavior (a custom npm scope /
registry / access / postinstall, a different Go module path, a specific release
repo) and (b) **publish to places cljgo doesn't ship built in** — a Homebrew tap,
a Scoop bucket, an OCI/container registry, an internal artifact server, S3, apt.
Hard-coding every destination is a losing game.

`build.cljgo` is already the right home: it is a **program, not a data file** (the
Zig model, ADR 0021) — its `(defn build [b] …)` composes `exe`/`install`/`run`
today. Publishing belongs in that same program as first-class, configurable,
extensible steps, rather than as opaque top-level CLI commands with no seam.

## Decision

### 1. Publishers are adapters, configured in `build.cljgo`

A **publish adapter** is a named target that takes the built artifacts and ships
them somewhere. The built-ins — `:go`, `:npm`, `:dist` — become adapters with the
same surface as any custom one, so overriding them and adding new ones are the
same mechanism.

```clojure
(defn build [b]
  (let [app (exe b {:name "todo" :main "src/todo/core.cljg"})]
    (install b app)
    ;; configure/override a built-in adapter:
    (publish b app :npm {:scope "@you" :repo "you/todo"
                         :registry "https://registry.npmjs.org" :access :public})
    (publish b app :go  {:module "github.com/you/todo"})))
```

`cljgo publish <adapter>` dispatches to the adapter configured in the plan;
`cljgo publish` with no arg lists the configured adapters; `cljgo publish all`
runs every one. A built-in invoked with no `build.cljgo` config keeps today's
zero-config defaults (ADR 0054/0079) — configuration is override, not obligation.

### 2. Custom adapters — publish anywhere

An adapter is just a function registered in the plan; `cljgo publish <name>` runs
it with a well-defined **publish context**:

```clojure
(defn brew-tap [ctx]
  ;; ctx: {:name :version :tag :artifacts [{:target "darwin/arm64" :path … :sha256 …} …]
  ;;       :checksums-file :repo :project-dir …}
  (let [f (render-formula ctx)]              ; a Homebrew formula from the dist artifacts
    (push-to-tap "you/homebrew-tap" f)))

(defn build [b]
  (publish-adapter b :brew brew-tap)         ; register -> `cljgo publish brew`
  (publish-adapter b :ghcr (oci-push-adapter {:registry "ghcr.io/you"}))
  …)
```

The **publish context** an adapter receives is the stable contract: the cross-
compiled artifacts (path + `GOOS/GOARCH` + sha256, straight from `dist`, ADR
0077), the checksums file, the version/tag, the project metadata, and the staging
dir. Given that, an adapter can produce a Homebrew formula, a Scoop manifest, a
container image, an upload to any registry/bucket — cljgo does not need to know
the destination, only to hand over the artifacts.

### 3. Override hooks on the built-ins

Beyond config maps, the built-in adapters expose hooks so a plan can customize
without replacing them: e.g. `:npm {:postinstall <fn-or-template>, :files […],
:before-publish <fn>}`, `:go {:before-tag <fn>}`, `:dist {:targets [...],
:archive :tar.gz}`. A hook that isn't provided uses the built-in default.

### 4. Staging + irreversibility respected

Every adapter stages its output (files written, commands printed) and performs
the irreversible remote step only under `--publish` (or an adapter-declared
confirmation) — the same outward-action discipline `publish go`/`npm` already
follow. A custom adapter gets a `ctx` flag (`:publish?`) so it honors the same
gate.

## Consequences

- Distribution becomes open-ended: cljgo ships `go`/`npm`/`dist` as the batteries,
  and any project extends to Homebrew/Scoop/OCI/S3/private registries by writing a
  small adapter fn in the `build.cljgo` it already has — no cljgo change, no
  plugin system beyond "it's a program."
- Overriding a built-in and adding a custom target are one uniform mechanism
  (`publish` + `publish-adapter`), so there is one thing to learn.
- The adapter contract is the `dist` artifact set (ADR 0077), which every target
  ultimately needs — so adapters compose cleanly on top of the cross-compile
  matrix rather than each re-deriving it.
- Keeps the Zig model coherent: the build (and now the publish) is code, versioned
  with the project, not a pile of external CI YAML.
- Roadmap ADR: ratifies the shape (adapters + publish context + override hooks +
  staging gate); the built-in refactor to adapters, the context contract, and a
  couple of reference custom adapters (brew, oci) land on their own spec/gates.
  ADR 0079's npm becomes the reference built-in adapter under this surface.
- Not chosen: a fixed enum of destinations baked into cljgo (the thing this
  replaces); an external plugin/registry system (unnecessary — `build.cljgo` is
  already executable and the natural extension point); doing remote publishes
  without the `--publish` gate.
