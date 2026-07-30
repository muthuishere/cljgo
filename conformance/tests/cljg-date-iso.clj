;; cljg.date/format-iso + parse-iso (ADR 0110 ask 4): the ISO-8601 / RFC 3339
;; instant every protocol on the wire carries. An instant is epoch MILLISECONDS
;; — the same integer cljg.date/now returns — so these compose with the clock.
;; Frozen here: whole seconds carry NO fractional part and sub-second instants
;; carry exactly three digits (java.time.Instant.toString's rule), the offset in
;; a parsed timestamp is honoured rather than dropped, round-trip identity, and
;; that a non-instant throws instead of returning a wrong number.
;;
;; `format` / `parse` (Go reference-time layouts) are deliberately NOT frozen
;; against the JVM: they have no JVM equivalent — a Go layout is not a
;; java.time pattern — so only their round-trip is pinned.
;;
;; oracle: clojure 1.12.5.1645, 2026-07-30 (java.time, the JVM's own instant):
;;   (str (java.time.Instant/ofEpochMilli 0))             => "1970-01-01T00:00:00Z"
;;   (str (java.time.Instant/ofEpochMilli 1500))          => "1970-01-01T00:00:01.500Z"
;;   (str (java.time.Instant/ofEpochMilli 1753900000123)) => "2025-07-30T18:26:40.123Z"
;;   (str (java.time.Instant/ofEpochMilli 1753900000000)) => "2025-07-30T18:26:40Z"
;;   (.toEpochMilli (java.time.Instant/parse "2026-07-30T12:00:00Z"))     => 1785412800000
;;   (.toEpochMilli (java.time.Instant/parse "2026-07-30T12:00:00.123Z")) => 1785412800123
;;   (.toEpochMilli (java.time.Instant/parse "2026-07-30T12:00:00+05:30")) => 1785393000000
;; The strings and longs below are those JVM values verbatim.
(require '[cljg.date :as date])
[(date/format-iso 0)
 (date/format-iso 1500)
 (date/format-iso 1753900000123)
 (date/format-iso 1753900000000)
 (date/parse-iso "2026-07-30T12:00:00Z")
 (date/parse-iso "2026-07-30T12:00:00.123Z")
 (date/parse-iso "2026-07-30T12:00:00+05:30")
 (= 1753900000123 (date/parse-iso (date/format-iso 1753900000123)))
 (date/format 1753900000123 "2006-01-02 15:04:05")
 (date/parse "2025-07-30 18:26:40" "2006-01-02 15:04:05")
 (try (date/parse-iso "not an instant") (catch Throwable e :threw))
 (try (date/parse "2025" "2006-01-02 15:04:05") (catch Throwable e :threw))
 (string? (date/format-iso))
 (format "%05.2f" 3.14159)]
;; expect: ["1970-01-01T00:00:00Z" "1970-01-01T00:00:01.500Z" "2025-07-30T18:26:40.123Z" "2025-07-30T18:26:40Z" 1785412800000 1785412800123 1785393000000 true "2025-07-30 18:26:40" 1753900000000 :threw :threw true "03.14"]
