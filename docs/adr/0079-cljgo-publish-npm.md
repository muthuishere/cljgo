# ADR 0079 — `cljgo publish npm`: ship a cljgo CLI to npm and Go, binary fetched from GitHub Releases

Date: 2026-07-24 · Status: proposed (roadmap; owner-directed: *"npm publish
golang publish, npm post install will download from github for all is built
in"*). Depends on ADR 0077 (`cljgo dist`), ADR 0054 (`cljgo publish go`), ADR
0078 (bri.cli). Second of the bri.cli block.

## Context

`cljgo dist` (ADR 0077) produces a native binary per platform + `checksums.txt`.
`cljgo publish go` (ADR 0054) publishes a library to the Go module ecosystem. The
missing distribution channel for a **CLI** is the one most users install from:
**npm** — `npm i -g todo` / `npx todo` is how a huge audience expects to get a
command-line tool, regardless of what it's written in. The proven pattern (esbuild,
`@biomejs/biome`, `swc`) is a thin npm package whose **postinstall** fetches the
correct prebuilt native binary for the host platform. The owner wants this **built
in**: one command, both ecosystems, the GitHub-Releases-backed download wired for
you.

## Decision

### 1. `cljgo publish npm` — one command, the whole flow

```
cljgo publish npm [--tag v1.2.3] [--repo owner/name] [--scope @you] [--dry-run]
```

It (a) runs `cljgo dist` for the npm-relevant matrix, (b) creates/uploads a
**GitHub Release** for the tag with every binary + `checksums.txt` as assets
(via `gh release`), and (c) generates and publishes the **npm wrapper package**.
`cljgo publish go` stays as-is (ADR 0054); a convenience `cljgo publish all`
runs both. Everything is staged/printed unless `--publish` is passed (the
owner's outward-action rule — publishing is irreversible).

### 2. The npm wrapper package (generated, not hand-written)

A minimal, generated package:

- `package.json` — `name`, `version` (= the dist tag), `bin` mapping the command
  name to a launcher, an `os`/`cpu` hint, and a `postinstall` script.
- `bin/<cmd>` — a tiny Node launcher that `exec`s the downloaded native binary
  (resolved from the package's install dir), passing argv through untouched.
- `postinstall.mjs` — detects `process.platform`/`process.arch`, maps to the
  `GOOS/GOARCH` asset name (`<name>_<os>-<arch>[.exe]`), **downloads it from the
  GitHub Release**, and **verifies it against `checksums.txt`** (the ADR 0077
  file — integrity is not optional) before marking it executable. A failed or
  offline install is a clear, actionable error, never a half-installed command.

The generated package is real files written to a staging dir (ADR 0047 spirit:
no string-literal scaffolding), reviewable before publish.

### 3. Two fetch strategies, postinstall-download the default

- **Default — postinstall download** (what the owner described): one published
  npm package; the binary is fetched from the GitHub Release on install. Simplest
  to publish and understand.
- **Alternative — `--optional-deps`** (the esbuild model): one per-platform
  package (`@scope/cmd-linux-amd64`, …) each shipping its binary, selected by
  npm's `os`/`cpu` `optionalDependencies`, plus a launcher package. No network in
  postinstall (more robust in locked-down CI), at the cost of publishing N
  packages. Offered as a flag; the download model is the default.

### 4. Integrity + provenance

The download is checksum-verified against the release's `checksums.txt`; the
tag, repo, and asset names are derived from `dist` output so the npm package and
the Go release describe the *same* artifacts. (npm provenance / sigstore
attestation is a future add-on, noted not decided.)

## Consequences

- A cljgo CLI reaches both audiences from one command: `go install …@latest` for
  the Go world, `npm i -g …` / `npx …` for everyone else — the binary is the same
  cross-compiled artifact `cljgo dist` already produces, just delivered two ways.
- No hand-rolled release plumbing: the GitHub Release, the platform mapping, the
  postinstall, and the checksum verification are generated and wired by cljgo.
- Depends only on tooling already present (`dist` + `gh`); Node is needed only by
  the *consumer* of the npm package, never by cljgo itself.
- Roadmap ADR: ratifies the shape (dist → GitHub Release → generated npm wrapper,
  checksum-verified, download-by-default) and the command surface; the generator,
  the two strategies, and release automation land on their own spec/gates.
- Not chosen: bundling the binary inside the npm tarball for every platform (huge,
  and npm discourages it); a Homebrew/Scoop/apt matrix in v1 (natural follow-ons
  once the Release-assets pipeline exists — they consume the same artifacts).
