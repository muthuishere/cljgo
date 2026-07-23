;; default-data-readers + *default-data-reader-fn* + *data-readers*
;; precedence (fundamentals batch A2, completing ADR 0050's tagged-literal
;; machinery): a *data-readers* entry overrides even the built-in
;; #inst/#uuid tags; *default-data-reader-fn* is the LAST resort — never
;; consulted for a known tag.
;; oracle (clojure 1.12.5, `clojure -M` 2026-07-23):
;;   (sort (map key default-data-readers)) => (inst uuid)
;;   (binding [*default-data-reader-fn* (fn [t v] [t v])] (read-string "#foo/bar 1")) => [foo/bar 1]
;;   (binding [*data-readers* {'inst (fn [v] [:inst v])}] (read-string "#inst \"2020-01-01\"")) => [:inst "2020-01-01"]
;;   (binding [*default-data-reader-fn* (fn [t v] :ddr)] (pr-str (read-string "#inst \"2020-01-01T00:00:00.000-00:00\""))) => "#inst \"2020-01-01T00:00:00.000-00:00\""
;;     (a fully-written timestamp, because the JVM re-prints a Date in that
;;     normalized form while cljgo's Inst round-trips the literal verbatim —
;;     with this spelling both agree byte-for-byte)
;;   ((get default-data-readers 'uuid) "290b4a86-2f5a-4bcf-b6d6-fbafb3d971ea") reads a real uuid value
;;   *read-eval* => true (the default; cljgo's reader has no #= at all —
;;   the safe subset of every *read-eval* setting)
[(sort (map key default-data-readers))
 (binding [*default-data-reader-fn* (fn [t v] [t v])] (read-string "#foo/bar 1"))
 (binding [*data-readers* {'inst (fn [v] [:inst v])}] (read-string "#inst \"2020-01-01\""))
 (binding [*default-data-reader-fn* (fn [t v] :ddr)] (pr-str (read-string "#inst \"2020-01-01T00:00:00.000-00:00\"")))
 (pr-str ((get default-data-readers 'uuid) "290b4a86-2f5a-4bcf-b6d6-fbafb3d971ea"))
 *read-eval*]
;; expect: [(inst uuid) [foo/bar 1] [:inst "2020-01-01"] "#inst \"2020-01-01T00:00:00.000-00:00\"" "#uuid \"290b4a86-2f5a-4bcf-b6d6-fbafb3d971ea\"" true]
