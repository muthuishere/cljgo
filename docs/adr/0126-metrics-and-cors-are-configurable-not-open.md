# 0126 — `/metrics` and CORS are configurable, and closed by default

Date: 2026-08-08 · Status: accepted (owner-directed) · Breaking: yes, and
deliberately — cljgo is 0.x

## Context

A bri capability audit found two defaults that are safe in dev and wrong
everywhere else, both reached through `api-defaults`, i.e. on for every
app that calls `http/listen`:

1. **`/metrics` was open.** `ops-routes` took a `:metrics-guard` that
   defaulted to `nil`, and the docstring said so out loud — *"DEFAULT IS
   OPEN; GUARD IT IN PROD"*. Neither `api-defaults` nor `listen` set one.
   A documented footgun is still a footgun: the endpoint exposes every
   route's traffic volume and latency distribution, which is an
   reconnaissance map of the application.
2. **CORS defaulted to `origin: *` in every profile.** The docstring
   called this "the dev default", but the code never checked the profile,
   so a production API shipped permissive CORS unless the operator knew
   to set `:origins`.

Both are the same shape: the secure configuration existed and was
documented, but the *default* was the insecure one, so safety depended on
the operator having read the docstring.

The owner ruled: **do not leave it unauthenticated, make it configurable —
and cljgo is 0.x**, so a breaking change is the right instrument rather
than a compatibility shim.

## Decision

Both fail **closed**, and both stay configurable in one obvious place.

**`/metrics`** — `:metrics-guard` selects the posture:

| `:metrics-guard` | behaviour |
|---|---|
| absent, `APP_METRICS_TOKEN` set | requires `Authorization: Bearer <token>`, constant-time compare |
| absent, no token | **route is not mounted** outside dev; a line on stdout says exactly how to enable it |
| a middleware | your own gate (e.g. `(auth/admin-only)`) — unchanged behaviour |
| `:public` | mounted open, deliberately, for a private network or sidecar scrape |

In dev (`BRI_DEV=1`) with nothing configured it stays open, because a
local Prometheus scrape is the normal case and a dev who has to configure
a token to see their own metrics will simply set `:public` forever.

A failed token check returns **404, not 401**, so a scan cannot confirm
the endpoint exists. This costs nothing (the operator knows the path) and
denies a scanner the confirmation a 401 hands over.

**CORS** — `:origins` (or `APP_HTTP__CORS_ORIGINS`) is the allowlist:

| configuration | behaviour |
|---|---|
| set | allowlist, echoing only an origin it allows |
| unset, dev | permissive `*`, as before |
| unset, not dev | **no CORS headers emitted at all** |

Emitting nothing is the correct answer for an API that never declared
which origins it trusts: the browser's own same-origin policy refuses the
request, and server-to-server and CLI callers are unaffected because they
ignore CORS entirely. `:origins "*"` still works — permissive on request,
never by omission.

## Consequences

- **This breaks running apps, which is the point.** An app that relied on
  the open `/metrics` now 404s until it sets `APP_METRICS_TOKEN` or
  `:metrics-guard :public`; a browser app that relied on implicit `*` CORS
  must name its origins. Both are one line, both are named in the startup
  message or the docstring, and 0.x is when this is cheap to do.
- The failure mode is a **loud 404 or a blocked browser request**, not
  silent data exposure. That is the right direction for the mistake to
  point.
- Frozen in `conformance/tests/bri-http-secure-ops-defaults.clj` (both
  harness legs). It asserts only the env-independent postures, because a
  conformance test that depends on `BRI_DEV` or `APP_METRICS_TOKEN` being
  unset in the runner's environment would be a flake waiting to happen;
  the token and dev paths are covered in `pkg/bri`'s Go tests.
- `rate-limit` and `auto-ban` keep their process-local stores and are
  still per-pod behind a load balancer. That is a *different* defect
  (documented in both docstrings) and is not addressed here.
- Measured behaviour, all six postures, before merge:
  unconfigured prod → 404 (not mounted) · `:public` → 200 · token set +
  no header → 404 · wrong token → 404 · right token → 200 · dev
  unconfigured → 200. CORS: prod unconfigured → no header · prod explicit
  `*` → `*` · allowlist → echoes only the allowed origin, never the
  caller's · dev unconfigured → `*`.
