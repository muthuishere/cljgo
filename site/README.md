# cljgo site

The project landing + getting-started page for **cljgo** (Clojure hosted on Go).

- `index.html` — a single, self-contained, responsive, dark/light-aware page.
  No external assets, no CDN, no JavaScript dependencies, no trackers — all CSS
  is inlined so it renders anywhere (including offline).

Content is derived from and kept consistent with the repo: `README.md`,
`cmd/cljgo/main.go` (the real CLI surface), `go.mod` (module path + Go 1.26),
`design/00-architecture.md`, `design/08-build-comptime-compat.md`, and the
snippets in `examples/`.

## Preview locally

```sh
cd site
python3 -m http.server 8000
# open http://localhost:8000
```

(Append `?theme=dark` or `?theme=light` to force a theme when previewing.)

## Deploy

Deployment is automated by `.github/workflows/pages.yml` (modern
Pages-via-Actions flow) on every push to `main` that touches `site/`, and via
manual `workflow_dispatch`. It publishes **only** this `site/` directory, so it
never clobbers the `docs/` ADR folder.

**One-time setup:** repo **Settings → Pages → Source: "GitHub Actions"**. After
that the workflow builds and deploys automatically.
