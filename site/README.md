# cljgo site

The public site at https://muthuishere.github.io/cljgo/ — two parts, one deploy:

- **Landing page** — `public/index.html`, hand-written, self-contained. Served at
  the site root exactly as authored (Astro copies `public/` verbatim).
- **Docs** — Astro + [Starlight](https://starlight.astro.build/) pages in
  `src/content/docs/`, themed to match the landing page
  (`src/styles/theme.css`: Go-cyan + Clojure-green on charcoal, Inter +
  JetBrains Mono). Search (Pagefind) and `llms.txt` / `llms-full.txt`
  (via `starlight-llms-txt`) come out of the build automatically.

## Comments

Every docs page ends with a public comment box
(`src/components/Footer.astro`) — [giscus](https://giscus.app) backed by
GitHub Discussions on `muthuishere/cljgo` (category **Announcements**,
mapped by pathname). Readers sign in with GitHub; threads are visible to
everyone under the repo's Discussions tab.

One-time setup (already done unless recreating the repo): enable
Discussions in repo settings and install the giscus GitHub App on the repo
(https://github.com/apps/giscus).

## Develop

```sh
cd site
npm install
npm run dev      # http://localhost:4321/cljgo/
npm run build    # writes dist/
```

## Deploy

`.github/workflows/pages.yml` builds the site and publishes `site/dist` to
GitHub Pages on every push to `main` touching `site/**`. One-time repo
setting: Settings → Pages → Source = "GitHub Actions".

## Conventions

- Internal doc links use the base-absolute form `/cljgo/<slug>/`.
- Numbers and claims in docs are copied from repo sources (README,
  conformance, benchmarks) — never invented. Keep it that way.
- Adding a page: create it under `src/content/docs/` and add its slug to
  the sidebar in `astro.config.mjs`.
