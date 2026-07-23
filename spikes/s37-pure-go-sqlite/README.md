# S37 — pure-Go SQLite as cljgo's zero-install default DB (the `bun:sqlite` model)

ADR 0041 chose **embedded Postgres** for the zero-install dev database,
and S25 (bri's data layer) flagged that as an open owner call: option
(a) vendor a zonky-style embedded-postgres downloader (~100MB, real
parity), (b) require Docker (not zero-install), (c) ship an SQLite dev
fallback (zero-install, breaks prod parity). S25 recommended (a) but
never proved it. The owner has since reversed toward **(c) — a
Bun-style zero-install SQLite default**, with production upgrading to
Postgres. This spike tests whether pure-Go SQLite is good enough to
bless.

cljgo's identity is a **single static binary, `CGO_ENABLED=0`**
(`.goreleaser.yaml`). That instantly disqualifies `mattn/go-sqlite3`
(the popular driver — it needs cgo). The only cgo-free option is
**`modernc.org/sqlite`** (a machine-translation of SQLite's C amalgam
to pure Go). Everything below is about whether that specific package is
good enough.

## Exit criteria (written BEFORE any code)

1. **Static build proof.** Does `modernc.org/sqlite` build and run
   under `CGO_ENABLED=0`? Prove it: `CGO_ENABLED=0 go build`, then
   `file ./bin` (expect statically-linked / no cgo) and `otool -L ./bin`
   (macOS) — confirm NO dynamic `libsqlite3` linkage. Yes/No.
2. **Binary-size delta.** Build a trivial hello binary WITH and WITHOUT
   the sqlite import; report the MB the battery adds.
3. **Latency + throughput, honest numbers.** Startup + first-query
   latency; single-row insert/sec and read/sec (prepared); and a bulk
   insert inside one transaction (rows/sec). No cherry-picking.
4. **One API over two drivers.** Does a SINGLE `bri.db` verb set
   (`query` / `one` / `insert` / `tx`, plain maps out, snake_case ↔
   kebab-case names) work over BOTH sqlite and pgx? Show the SQLite side
   REALLY running against a file DB and a `:memory:` DB; the pgx side may
   be stubbed (S25 already proved pgx).
5. **Single-binary deploy story.** Does the file-DB survive the
   single-binary deploy — DB file next to the binary, `.dev/` data dir?
   Any WAL / locking gotchas for a server (concurrent) workload?

## Non-goals

Not building `bri.db`. Not touching `pkg/`, `core/`, `cmd/`,
`conformance/`, or the root `go.mod`. Probe is a throwaway module.

## Layout

- `probe/` — standalone Go module (`module cljgospike/s37`, own go.mod),
  carrying: the static-build harness, the size-delta hello binaries, the
  latency/throughput benchmarks, a driver-agnostic `bri.db`-shaped
  adapter running over `modernc.org/sqlite` against both a file DB and
  `:memory:`, and the WAL/concurrency probe.
- `shapes/` — the blessed `bri.db` API sketch as real Clojure, shown
  identical across the sqlite default and a pgx prod upgrade.
- `run.sh` — builds everything static, runs it, prints PASS/FAIL per
  criterion and the size delta + throughput table.
- `VERDICT.md` — verdict, evidence, blessed form, un-proven risks, owner
  call.
