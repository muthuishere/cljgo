;; bean over Go struct reflection (tail wave, 2026-07-23) — the
;; cljgo-truthful analogue of the JVM's JavaBean getter reflection:
;; exported fields of a Go struct (or pointer to one) become a read-only
;; map with kebab-cased keyword keys (RawQuery -> :raw-query). DEVIATIONS
;; (documented): no :class entry, unexported fields skipped, no getters
;; involved — there are no JavaBeans on a Go host. JVM shape cited
;; (clojure 1.12.5): (bean (java.util.Date. 0)) => {:class java.util.Date
;; :time 0 ...} — a map of getter-derived keys.
;; oracle: skip — reflects GO structs; no JVM equivalent value exists
;; (JVM bean shape cited above)
(require-go '[net/url])
(def u (url/URL. {:Scheme "https" :Host "x" :RawQuery "a=1" :ForceQuery true}))
[(select-keys (bean u) [:scheme :host :raw-query :force-query :omit-host])
 (map? (bean u))
 (try (bean 5) (catch Exception e :not-a-struct))]
;; expect: [{:scheme "https", :host "x", :raw-query "a=1", :force-query true, :omit-host false} true :not-a-struct]
