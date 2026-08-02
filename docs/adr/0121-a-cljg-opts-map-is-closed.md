# ADR 0121 — a `cljg.*` opts map is closed, and an unknown key is a coded error

- Status: accepted
- Date: 2026-08-02
- Relates to: ADR 0120 (`:timeout-ms` is the name), ADR 0015 (diagnostics),
  ADR 0071 (shim installation), issue #192
- **Breaking** for callers who pass keys a `cljg.*` function does not declare

## Context

ADR 0120 fixed an instance and named the class it left open:

> Any misspelled opts key in any `cljg.*` namespace still takes the default in
> silence.

That is the defect. `(:timeout-ms opts 30000)` means a misspelled option has
**no symptom**: the call succeeds, the caller's intent evaporates, and a
timeout that never applies still returns a correct response from a fast
server. koine's HTTP timeout was dead in production while their suite was
green on both hosts — and so was ours, because a silent default cannot be
observed by an assertion that the call worked.

There is nothing HTTP-specific about it. Every `cljg.*` opts map reads its
keys the same way, so every one of them has the same hole: `cljg.io/exec`
`:timeout` (the majority spelling is `:timeout-ms`), `cljg.process/spawn`
`:timeout-ms` (which that function does not have at all — it has only `:env`
and `:dir`), `cljg.cache/local` `:ttl-ms`, `cljg.socket/dial` `:tls-config`.
Each is a program that runs and quietly does something other than what it
says.

## Decision

**1. The option set of a `cljg.*` function is CLOSED. An unrecognised key
raises G5027 with a did-you-mean `Fix`.**

```
error: cljg.net.http/request: unknown option :time-out (known: :method :url :headers :query :body :json :edn :form :timeout-ms :timeout :retry :as)
help: did you mean :timeout?
help: run `cljgo explain G5027`
```

**2. One mechanism, one code, one message.** A single Go function,
`pkg/bri/opts_check.go`, interned as the private `-check-opts` var into
**every** bri/cljg namespace (one line in `InstallShimsInto`). The `.cljg`
files call it as the first form of each public function, passing the allowed
key set as a literal that sits beside the docstring documenting the same
keys — so the check and the documentation cannot drift across a file
boundary.

There is deliberately no validator registry, no spec layer, no strategy
object and no way to configure it. The whole mechanism is one function, one
shim and 26 call sites, and if it were the same speed with a plugin
architecture we would still not want the plugin architecture.

**3. The check is one level deep.** It validates the opts map itself and
never descends into a value. Nested maps in a `cljg.*` opts map are data
whose keys belong to the caller — `:headers`, `:query`, `:form`
(`cljg.net.http`), `:env` (`cljg.io/exec`, `cljg.process/spawn`) — and a
recursive check would reject perfectly good programs.

**4. Two maps stay OPEN, on purpose.** `cljg.os/job` and `cljg.os/service`
are `(merge {…} opts)`: the caller's map *becomes* the job/service record and
is carried, not consumed. Extra keys there are the documented idiom, not a
mistake, and closing those two would break working programs to fix nothing
(there is no silent default to lose). `TestOpenOptsMapsStayOpen` states this
out loud so a later change cannot close them by accident.

**5. The did-you-mean lives at the render layer, not the raise site.** The
raise site owns the words and the registered code; `pkg/diag`'s
`applyUnknownOpt` owns the `Fix` — the split `applyJavaStatic` already
documents. That is not stylistic. Computing the `Fix` at the raise site would
mean `pkg/bri` importing `pkg/diag`, which imports `pkg/reader`, which would
then link into **every compiled bri binary** for the sake of one suggestion
string. `pkg/bri`'s import list stays `pkg/lang`, and the message carries the
known set inline so the render layer has everything it needs.

## The breaking half, argued rather than assumed

An open map is a legitimate Clojure idiom, and threading one map through
several functions is ordinary practice. This decision refuses that **for
`cljg.*` opts maps specifically**, and the argument is that they are not
context maps — they are an argument list written in map syntax. Three
consequences follow:

- **A caller who threads one map through `cljg.io/exec` and
  `cljg.net.http/request` breaks.** They must `select-keys` (or build the two
  argument lists separately). We accept that, because the two functions share
  exactly one key (`:timeout-ms`) and disagree about every other one: today
  such a caller is *already* silently losing `:in`/`:dir`/`:env` at the HTTP
  call and `:headers`/`:retry`/`:as` at the exec call. The threaded map was
  never doing what it looked like it was doing. Making that visible is the
  point, not collateral damage.
- **A caller who deliberately parks extra data on an opts map breaks.** This
  is the real cost, and it is not zero. We take it because the alternative —
  accept unknown keys, warn somewhere — is the status quo with extra
  paperwork: a warning on a server path is a log line nobody reads, and the
  failure it is warning about is still invisible in every test.
- **`:timeout` stays in `cljg.net.http`'s known set forever** (ADR 0120). A
  closed key set that closed on the wrong keys would be a worse defect than
  the one it replaces, so `TestKnownOptsStillPass` pins both spellings.

Rejected alternatives:

- *Warn instead of refuse.* Keeps the program running and the defect
  invisible. The whole finding of #192 is that "still runs" was the problem.
- *A spec/schema layer per namespace.* Types the values as well as the keys —
  and buys a second code path, a registry and an invalidation story for a
  defect that is entirely about a misspelled keyword. Refused under
  *simplicity before performance*.
- *Check in each Go shim instead.* Would work for the six namespaces whose
  opts map reaches Go whole, and not at all for `cljg.net.http/request`,
  `cljg.compress`, `cljg.cache` or `cljg.jobs`, which read their options in
  Clojure. Two mechanisms for one rule is worse than one mechanism in the
  place both can reach.

## Scope — what is fixed and what is not

Fixed (26 call sites): `cljg.net.http/request` (and every verb, which
forwards to it), `cljg.http/serve`, `cljg.socket/listen|dial|udp-listen|
udp-recv`, `cljg.io/exec|write-bytes` (and `sh!`, which forwards),
`cljg.process/spawn`, `cljg.stream/to-file`, `cljg.compress/gzip|gunzip|
deflate|inflate|zlib-compress|zlib-decompress`, `cljg.cache/local`,
`cljg.jobs/local`, `cljg.os/run`, `cljg.data.cast/connect`,
`cljg.security/sign|verify|issue|save-to-keychain|get-from-keychain|
delete-from-keychain|auto-ban`, and `bri.web.openapi/client` — the one
`bri.*` function #192 names by hand, because it carries the same `:timeout`
alias.

**NOT fixed, and named so it is not mistaken for done:**

- **The rest of `bri.*`.** `bri.web.http`, `bri.cli`, `bri.cli.api` and
  `bri.core.config` have the same hole. `-check-opts` is interned in every
  namespace, so closing them is one line each — but `bri.web.http`'s maps are
  Ring request/response maps and route options tangled together, and deciding
  which of those are closed is a separate reading job, not a rename.
- **`cljg.secrets`' `{:service :name :value}` map-spec arguments.** Same
  class, deliberately deferred: those are positional arguments in map syntax
  and a missing key already fails loudly, so the silent-default defect this
  ADR is about does not arise there.
- **Value types.** `{:retry "3"}` is still accepted here and still wrong.
  This ADR closes the key set only.
- **`:timeout` in `cljg.net.http` is still a second name for one concept.**
  ADR 0120 argued that cost; a closed key set now makes removing it *possible*
  (it would fail loudly rather than silently restoring the default) but not
  obviously worth the breakage.

## Consequences

- A misspelled option now fails at the call site with the function named, the
  key named, the full known set listed, and a suggestion when one is within
  two edits. The koine defect could not have shipped.
- Programs passing undeclared keys to the listed functions stop working. That
  is the intended effect and it is why this is a `BREAKING` note in the
  release, not a fix note.
- Cost per call is a linear scan of the keys present against a shared
  top-level vector — no allocation, no map construction, and the vector is
  `def`'d once rather than rebuilt per request.
- The AOT twin (`pkg/briaot/*`) is regenerated, so interpreted and compiled
  refuse identically. `-check-opts` is interned by `InstallShimsInto`, which
  both loaders call, so there is one implementation and no parity to hope for.

## Verification

- `pkg/bri/opts_check_test.go` — `TestUnknownOptIsRefused` (14 namespaces,
  one misspelling each), `TestUnknownOptCarriesCodeAndDidYouMean` (G5027 +
  the `:timeoutms` → `:timeout-ms` `Fix` + the rendered help lines),
  `TestKnownOptsStillPass` (the documented options, both timeout spellings,
  and `nil` opts), `TestOpenOptsMapsStayOpen` (`cljg.os/job` keeps a caller
  key).
- **Confirmed failing before the change**, by making `-check-opts` a no-op —
  which is exactly the old semantics. Every subtest of
  `TestUnknownOptIsRefused` failed with `expected an error, got none`, and
  the diagnostics test with `a misspelled option returned normally — it was
  silently defaulted`. That is the defect printing its own description.
- Registry, explain page and site row are enforced in sync by
  `pkg/diag`'s `TestRegistryLockMatches`, `TestEveryCodeHasAnExplainPage` and
  `TestSiteDiagnosticsPageListsEveryCode`.
