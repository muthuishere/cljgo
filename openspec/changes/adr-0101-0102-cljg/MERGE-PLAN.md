# Merge plan — ADR 0101 + ADR 0102 (cljg.* stdlib expansion)

Three worktrees, each built green off `main` in isolation. Every task
independently edited `pkg/bri/bri.go` `Specs()` and regenerated `pkg/briaot`
— **those two touchpoints CONFLICT** and must be stitched by hand: apply all
Spec rows / renames ONCE, then run a SINGLE `go generate ./pkg/briaot`. Do not
take any worktree's regenerated `pkg/briaot/**` as-is.

This stacks on the already-committed **`feat/clojure-contrib-tier1`**, which
appended 4 contrib Spec rows at the end of `Specs()`. The stitched final
`Specs()` order is:

1. all existing `bri.*` / `cljg.*` rows — **with the four `bri.core.{cache,jobs,secrets,data}` rows RENAMED in place to `cljg.*`** (ADR 0102),
2. the 4 `feat/clojure-contrib-tier1` contrib rows (unchanged),
3. the 5 NEW `cljg.*` rows from ADR 0101 (`system`, `date`, `stream`, `process`) appended LAST, in that order, after the contrib rows.

> Ordering rule: the renamed 0102 rows stay in their CURRENT positions
> (rename only Name/File/Pkg/Source; do not move them) so genbri's gensym
> numbering for earlier-emitted namespaces is untouched. All ADR-0101 rows are
> non-requiring leaves — append them strictly LAST so they never shift the
> gensym numbering of anything emitted before them.

---

## Task A — ADR 0101 leaves: `cljg.system` + `cljg.date`

**Green:** yes.

### New files (safe to copy verbatim)
- `core/cljg/system.cljg`
- `core/cljg/date.cljg`
- `pkg/bri/cljg_system.go`
- `pkg/bri/cljg_date.go`
- `conformance/tests/cljg-system-getenv.clj`
- `conformance/tests/cljg-date-monotonic.clj`
- `docs/adr/0101-cljg-system-and-date.md`

### Non-generated edits to re-apply by hand
- `core/cljg.go` — add `CljgSystemSource`, `CljgDateSource` embeds.
- `pkg/briloader/briloader.go` — **infra fix (shared, prerequisite):**
  `providerLoad`'s loaded guard must ALSO require `lang.FindNamespace != nil`,
  so a provider-backed namespace re-required after per-file teardown reloads
  instead of no-opping. **This same file is also edited by Task C** — see
  Conflict note below; both edits must survive.

### Specs() rows to ADD (append last, after contrib rows)
```go
{Name: "cljg.system", File: "cljg/system.cljg", Pkg: "cljgsystem", Source: &core.CljgSystemSource, install: installSystemShims},
{Name: "cljg.date",   File: "cljg/date.cljg",   Pkg: "cljgdate",   Source: &core.CljgDateSource,   install: installDateShims},
```
Installers `installSystemShims` / `installDateShims` come from
`pkg/bri/cljg_system.go` / `cljg_date.go`.

### Regenerated (DO NOT copy — produced by the single regen at the end)
- `pkg/briaot/briaot.go`, `pkg/briaot/cljgsystem/`, `pkg/briaot/cljgdate/`

### Callers rewritten
None.

---

## Task B — ADR 0101 streaming: `cljg.stream` + `cljg.process` + `cljg.net.http :as :stream`

**Green:** yes.

### New files (safe to copy verbatim)
- `core/cljg/stream.cljg`
- `core/cljg/process.cljg`
- `pkg/bri/stream.go`
- `pkg/bri/proc_spawn.go`
- `pkg/bri/stream_test.go`
- `cmd/cljgo/stream_compiled_test.go`
- `docs/adr/0101-cljg-process-streaming.md`
- `spikes/s56-stream-handle/VERDICT.md`

### Non-generated edits to re-apply by hand
- `core/cljg.go` — add `CljgStreamSource`, `CljgProcessSource` embeds.
- `core/cljg/net_http.cljg` — add opt-in `:as :stream` handling.
- `pkg/bri/net_http.go` — add the `-http-stream` shim.
- `pkg/bri/io_test.go` — streaming-related test additions (merge, don't
  clobber pre-existing io tests).

### Specs() rows to ADD (append last, after Task A's rows)
```go
{Name: "cljg.stream",  File: "cljg/stream.cljg",  Pkg: "cljgstream",  Source: &core.CljgStreamSource,  install: installStreamShims},
{Name: "cljg.process", File: "cljg/process.cljg", Pkg: "cljgprocess", Source: &core.CljgProcessSource, install: installProcSpawnShims},
```
`read-line` is `:exclude`'d from the `clojure.core` refer inside
`core/cljg/stream.cljg` (precedence principle) — preserved by copying the file.

### Regenerated (DO NOT copy)
- `pkg/briaot/briaot.go`, `pkg/briaot/cljgstream/`, `pkg/briaot/cljgprocess/`,
  `pkg/briaot/cljgnethttp/` (the `net_http` shim change re-emits this twin)

### Callers rewritten
None.

---

## Task C — ADR 0102 rename: `bri.core.{cache,jobs,secrets,data}` → `cljg.*`

**Green:** yes. This is a pure rename (protocols / impls / keywords preserved),
but it has the widest blast radius and the riskiest interaction with A/B.

Renames:
- `bri.core.cache`  → `cljg.cache`
- `bri.core.jobs`   → `cljg.jobs`
- `bri.core.secrets`→ `cljg.secrets`
- `bri.core.data`   → `cljg.data.cast`  *(see Semantic risk below)*

### New / moved source files (copy)
- `core/cljg/cache.cljg`      (moved from `core/bri/cache.cljg`)
- `core/cljg/jobs.cljg`       (moved from `core/bri/jobs.cljg`)
- `core/cljg/secrets.cljg`    (moved from `core/bri/secrets.cljg`)
- `core/cljg/data_cast.cljg`  (moved from `core/bri/db.cljg`)
- Delete the four old `core/bri/{cache,jobs,secrets,db}.cljg`.

### Non-generated edits to re-apply by hand
- `core/cljg.go` — add `Cljg{Cache,Jobs,Secrets,DataCast}Source` embeds
  (moved out of `core/bri.go`; remove the old `Bri*Source` embeds there).
- `core/bri.go` — drop the four moved embeds.
- `pkg/bri/secrets/secrets.go` — `RegisterInstaller` key → `cljg.secrets`
  (package name kept).
- `pkg/bri/db/db.go` — `RegisterInstaller` key → `cljg.data.cast`
  (package name kept).
- `pkg/briloader/briloader.go` — **also edited by Task A.** Merge both: Task A's
  `FindNamespace != nil` guard AND Task C's rename references must both land.
- `pkg/emit/module_test.go` — expected transitive-link Pkg `brisecrets` →
  `cljgsecrets`.
- `pkg/briaot/optin_linking_test.go` — old package paths → new.

### Specs() rows to RENAME IN PLACE (do NOT move — keep gensym order)
```go
{Name: "cljg.data.cast", File: "cljg/data_cast.cljg", Pkg: "cljgdatacast", Source: &core.CljgDataCastSource, install: nil, OptIn: true, ShimImport: "github.com/muthuishere/cljgo/pkg/bri/db"},
{Name: "cljg.secrets",   File: "cljg/secrets.cljg",   Pkg: "cljgsecrets",  Source: &core.CljgSecretsSource,  install: nil, OptIn: true, ShimImport: "github.com/muthuishere/cljgo/pkg/bri/secrets"},
{Name: "cljg.cache",     File: "cljg/cache.cljg",     Pkg: "cljgcache",    Source: &core.CljgCacheSource,    install: nil},
{Name: "cljg.jobs",      File: "cljg/jobs.cljg",       Pkg: "cljgjobs",    Source: &core.CljgJobsSource,     install: nil},
```
The old rows were `bri.core.data` (Pkg `bridb`), `bri.core.secrets`
(`brisecrets`), `bri.core.cache` (`bricache`), `bri.core.jobs` (`brijobs`).
Update only Name/File/Pkg/Source (+ they keep OptIn/ShimImport where present).

### Callers rewritten (all must be carried over — leaving any breaks `go test ./...`)
Clojure callers:
- `core/bri/audit.cljg`, `core/bri/cli_auth.cljg`, `core/bri/cli_api.cljg`

Go / cmd:
- `cmd/cljgo/generate.go`, `cmd/cljgo/migrate.go`, `cmd/cljgo/generate_test.go`,
  `cmd/cljgo/bri_test.go`, `cmd/cljgo/dbparity_test.go`,
  `cmd/cljgo/secretsparity_test.go`, `cmd/cljgo/cache_compiled_test.go`,
  `cmd/cljgo/jobs_compiled_test.go`, `cmd/cljgo/cli_api_compiled_test.go`
- `cmd/cljgo/testdata/dbparity.cljg`, `cmd/cljgo/testdata/secretsparity.cljg`
- `cmd/cljgo/resource_tmpl/{db.cljg.tmpl,resource.cljg.tmpl,resource_test.cljg.tmpl,migration.sql.tmpl}`

pkg tests:
- `pkg/bri/cache_test.go`, `pkg/bri/jobs_test.go`, `pkg/bri/secrets_test.go`,
  `pkg/bri/cast_test.go`, `pkg/bri/db_test.go`, `pkg/bri/cli_auth_test.go`,
  `pkg/bri/cli_api_test.go`, `pkg/bri/secrets/secrets_test.go`

Docs / site / examples / openspec (user-facing text, carry them so nothing ships stale):
- `examples/notes-db/{README.md,build.cljgo,src/app/main.cljg,test/app/main_test.cljg}`
- `docs/guides/{bri-db.md,bri-deploy.md,bri-tutorial.md,resource-generator.md}`
- `README.md`
- `site/astro.config.mjs`, `site/public/index.html`,
  `site/src/content/docs/bri/{tutorial.mdx,db.md,auth.md}`,
  `site/src/content/docs/guides/{deploy.md,generate.md}`,
  `site/src/content/docs/reference/roadmap.md`
- `openspec/changes/app-framework/tasks.md`

Left as history (do NOT rewrite): `docs/adr/*`, `openspec/changes/archive/*`.

### Regenerated (DO NOT copy — the single regen removes old dirs, emits new)
- `pkg/briaot/briaot.go`
- remove `pkg/briaot/{bricache,brijobs,brisecrets,bridb}/`
- emit `pkg/briaot/{cljgcache,cljgjobs,cljgsecrets,cljgdatacast}/`
- `pkg/briaot/briaudit/briaudit.go`, `pkg/briaot/bricliauth/bricliauth.go`
  (re-emitted because their Clojure requires changed)

---

## Conflict resolution (the three overlaps)

1. **`pkg/bri/bri.go` `Specs()`** — hand-merge: rename C's four rows in place,
   keep the contrib rows, append A's two then B's two rows last.
2. **`pkg/briloader/briloader.go`** — edited by A (FindNamespace guard) AND C
   (rename refs). Apply both hunks; they touch different lines.
3. **`core/cljg.go`** — edited by A (system/date), B (stream/process), and C
   (cache/jobs/secrets/data-cast moved in). Union all embed declarations;
   remove the four moved embeds from `core/bri.go`.
4. **`pkg/briaot/**`** — throw away all three worktrees' versions; produce fresh
   with the single regen below.

---

## Grep-gate (re-run after stitching, before regen)

Task C's gate — must return EMPTY across live tree:
```
grep -rn 'bri\.core\.\(cache\|jobs\|secrets\|data\)' core pkg templates examples conformance cmd site README.md docs/guides
```
(Expanded beyond C's original list to include `cmd`, `site`, `docs/guides`,
`README.md` — the real caller set. `docs/adr` and `openspec/changes/archive`
are history and legitimately still contain the old names.)

Old briaot package dirs must be gone:
```
ls pkg/briaot | grep -E 'bricache|brijobs|brisecrets|bridb'   # -> no matches
```

---

## Single regenerate + full gate (run ONCE, after all edits + grep-gate pass)

```
cd /Users/muthuishere/muthu/gitworkspace/clojure-workspace/cljgo
go generate ./pkg/briaot
CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates core && go test ./...
```
Key must-pass suites (all three tasks depend on them):
- `pkg/briaot` — `TestGeneratedBriIsUpToDate`, OptIn, `optin_linking_test`
- `pkg/coreaot` — `TestNoInterpreterInCompiledBinary`
- `pkg/bri` — cache/jobs/secrets/cast + stream/process + net-http-stream tests
- `cmd/cljgo` (compiled parity, `-timeout 1800s -p 1`) —
  dbparity, secretsparity, cache_compiled, jobs_compiled, cli_api_compiled,
  stream_compiled, net_http compiled parity
- `conformance` — `cljg-system-getenv`, `cljg-date-monotonic` (dual harness)

---

## Top risks

1. **Gensym cascade from the 0102 rename (highest).** genbri emits namespaces
   in `Specs()` order with positional gensyms. The four 0102 rows MUST be
   renamed strictly in place and the ADR-0101 rows appended strictly LAST; any
   reorder shifts gensym numbering and `TestGeneratedBriIsUpToDate` /
   `NoInterpreter` will churn or fail. Never copy a worktree's `pkg/briaot/**`
   — always regenerate from the stitched `Specs()`.
2. **Caller left behind (0102).** The rename surface is huge and spans
   `cmd/cljgo` generated-project templates + compiled parity fixtures + `site`
   + `README`. A missed reference either fails `go test ./...` or (worse for
   templates) silently ships stale `bri.core.*` in generated user code. The
   grep-gate above is the backstop — run it before regen.
3. **`briloader.go` double-edit.** Task A's `FindNamespace != nil` guard is a
   hard prerequisite for provider-backed namespaces to reload under per-file
   conformance teardown; Task C also edits this file. If only C's version lands,
   conformance for A's namespaces (and any provider-backed ns) goes red. Merge
   both hunks.
4. **Semantic flag (owner decision, not a merge defect):** `bri.core.data` was
   the ENTIRE blessed DB layer (connect/query/exec!/tx/migrate! …) PLUS the
   cast/cast! gate, renamed wholesale to `cljg.data.cast`. The full DB API now
   lives under a name that reads like only the cast gate. Faithful to the ADR
   0102 directive; if the owner wanted the DB layer under `cljg.data`/`cljg.db`
   with only the gate under `cljg.data.cast`, that is a follow-up split.
5. **ADR/spike number provenance.** ADR 0101 was authored fresh in both A and B
   as two separate files (`0101-cljg-system-and-date.md`,
   `0101-cljg-process-streaming.md`) — same number, different slugs. Decide
   whether to renumber one (e.g. 0102-streaming) before archiving; repo ADRs
   otherwise stop at 0096 and spikes at s49, so gaps 0097–0100 / s50–s55 remain.
   (0102 the app-framework move ADR is a third distinct doc.)
