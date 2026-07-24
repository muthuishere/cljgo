---
title: "bri.web.html"
description: "HTML as a function over data: hiccup-style vectors in, escaped HTML out — plus full documents, CSRF-carrying forms, and an explicit raw-HTML opt-out."
---

HTML is a **function** over data: hiccup-style vectors in, escaped HTML out. No template language, no layouts, no partials, no asset pipeline — `html/form` is the deliberate outer boundary of the surface, and CSS is a file in `public/` served by `(bri.web.http/dir "public")` (see [bri.web.http](/cljgo/bri/http/)).

## Elements

```clojure
(html/render [:h1 "hello"])                 ; => "<h1>hello</h1>"
(html/render [:a {:href "/x"} "go"])        ; attrs map second
(html/render [:p.hint#top "y"])             ; tag sugar: class/id
(html/render [:ul (for [x xs] [:li x])])    ; seqs splice
```

- Strings and numbers are text nodes, **escaped** — always.
- `nil` renders nothing (`when` compositions just work).
- Boolean attrs: `{:disabled true}` → `disabled`; `false`/`nil` omit.
- Void tags (`br`, `img`, `input`, `link`, `meta`, …) self-close per the HTML spec.

## XSS-safe by construction

Every text node and attribute value is escaped. The opt-out is explicit and ugly, on purpose:

```clojure
(html/unsafe-raw-html trusted-markup)   ; you own every byte
```

## Documents

```clojure
(html/page {:title "myapp"}          ; opts map optional
  [:main [:h1 "It's alive."]])
```

emits the full `<!doctype html>` document — charset, viewport, title, and the stylesheet link (default `/static/app.css`, override with `:stylesheet`).

## Forms carry the token

```clojure
(html/form {:post "/signup"}
  [:input {:name "email"}]
  [:button "Sign up"])
```

`html/form` **mints the CSRF token** (a hidden `__csrf` field) from the request the csrf middleware bound — which is why the browser POST passes the [default middleware stack](/cljgo/bri/http/). `{:get "/search"}` makes a GET form (no token needed); other opts keys pass through as attributes. It returns hiccup data: compose it, then `render`/`page` it.

JSON is equally first-class: return a map/vector body and the `negotiate` middleware encodes it (see [bri.web.http](/cljgo/bri/http/)).

## Where next

- [bri.web.http](/cljgo/bri/http/) — the handlers and middleware these pages flow through
- [bri.core.config](/cljgo/bri/config/) — profiles, env overlay, the schema
- [Tutorial](/cljgo/bri/tutorial/) — build the app these pieces belong to
