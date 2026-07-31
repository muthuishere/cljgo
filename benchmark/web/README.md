# The web benchmark — bri against the field

One command, eleven runtimes, real Docker containers, real HTTP over a socket.

```bash
bash benchmark/web/run.sh                     # the full table -> results.md
DURATION=30s CONC=100 bash benchmark/web/run.sh
```

Needs `docker`, [`oha`](https://github.com/hatoo/oha), and `curl`. Nothing
else — every server builds from source inside its own image.

## What it measures, and what it deliberately does not

Each runtime is built, cold-started, warmed, then load-tested on **both**
routes with `oha`, while peak RSS is sampled from `docker stats`. Image size
and cold-start come from Docker itself.

**One container at a time.** Contention between runtimes skews everything, so
nothing runs in parallel — the run is slow on purpose.

Every server answers the same two routes with **byte-exact** bodies, so no
entrant wins by sending less:

```
GET /            ->  hello\n                             (text/plain)
GET /api/hello   ->  {"msg":"hello from <runtime>"}      (application/json)
```

The cljgo entrant is the **flagship `bri.http`** (`bri/`), not a raw
`net/http` handler. Comparing our framework against someone else's framework
is the only honest shape; a bare Go handler is in the table as `go net/http`
precisely so you can see what the framework costs us.

**The bri image builds cljgo from THIS CHECKOUT**, not from a release. The
number you get is the number your working tree produces — which is the point,
if you are here to make it faster.

## What the numbers exclude

Say this out loud whenever you quote them:

- **A hello-world route.** No database, no template, no auth on the measured
  routes. This measures request-path overhead, and nothing else. A real app's
  profile is dominated by what the handler does.
- **Localhost, one machine.** No network, no TLS, no proxy, no cross-AZ
  latency. Real deployments add a term that dwarfs the differences here.
- **JVM entrants are warmed** (`WARM=3s`) but a 20-second run still does not
  reach a fully-JITted steady state. Their cold-start column is the honest
  one; their throughput column flatters a short run less than a long one.
- **Peak RSS is sampled**, not integrated. It is a ceiling observation, not
  an average.

## Adding a runtime

Drop a directory under `compare/` with a `Dockerfile` that serves the two
routes on `$PORT` (default 8080). `run.sh` discovers it — no edit here. Match
the response bodies exactly, or the comparison is meaningless.

## Where the published table lives

[`site/src/content/docs/reference/benchmarks.md`](../../site/src/content/docs/reference/benchmarks.md),
under "Web framework (bri) vs the field". Every table there carries its own
date and cljgo version, because they are not all measured at once. If you
re-run this, update that date — a stale benchmark quoted as current is a false
claim, not a rounding error.

The original investigation is spike `s45` (`spikes/s45-bri-aot-docker/`),
which is frozen. This is the living copy.
