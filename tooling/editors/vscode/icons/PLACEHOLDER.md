# Icon placeholders

`package.json` references three icons that do not exist yet:

- `cljgo.png` — 128×128 extension/marketplace icon
- `cljg-light.svg` / `cljg-dark.svg` — file icons for `.cljg` in the explorer

Design direction: the Clojure lambda-yin-yang silhouette recolored with the
Go gopher blue (`#00ADD8`) so `.cljg` files read as "Clojure family, Go
target" at a glance. Until real assets land, VS Code falls back to the
default file icon — the extension still works.
