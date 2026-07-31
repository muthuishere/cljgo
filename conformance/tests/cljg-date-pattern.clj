;; cljg.date/format-pattern + parse-pattern (ADR 0113): the java.time
;; DateTimeFormatter pattern language — the PORTABLE spelling, so a .cljc
;; library's :clj branch is DateTimeFormatter/ofPattern with no translation at
;; all. `format` / `parse` keep their GO reference-time layout meaning and are
;; deliberately untouched (ADR 0113 decision 6).
;;
;; Frozen here: the common patterns, the unpadded forms Go's layout language
;; cannot spell (H, m, s, M, d), text tokens at both widths, the meridiem,
;; quoted literals, the offset forms, round trips, and — the point of the whole
;; design — that an unrepresentable token is REFUSED rather than approximated.
;;
;; LOCALE IS ENGLISH, NOT ROOT. The oracle below pins Locale/ENGLISH; ROOT has
;; no distinct full forms and collapses MMMM to "Jul" and EEEE to "Fri", so a
;; caller following ROOT advice would diverge on exactly the tokens the advice
;; was written to protect.
;;
;; oracle: clojure 1.12.5 / java.time, 2026-07-31. Formatting, with
;;   (defn fmt [ms p] (.format (java.time.format.DateTimeFormatter/ofPattern
;;                               p java.util.Locale/ENGLISH)
;;                             (java.time.ZonedDateTime/ofInstant
;;                               (java.time.Instant/ofEpochMilli ms)
;;                               java.time.ZoneOffset/UTC))):
;;   (fmt 0             "yyyy-MM-dd HH:mm:ss")      => "1970-01-01 00:00:00"
;;   (fmt 1785488645123 "yyyy-MM-dd HH:mm:ss.SSS")  => "2026-07-31 09:04:05.123"
;;   (fmt 1785488645123 "EEE, d MMM yyyy")          => "Fri, 31 Jul 2026"
;;   (fmt 1785488645123 "EEEE d MMMM yyyy")         => "Friday 31 July 2026"
;;   (fmt 1785488645123 "h:mm a")                   => "9:04 AM"
;;   (fmt 1785488645123 "H:m:s")                    => "9:4:5"
;;   (fmt 1785488645123 "yy/M/d")                   => "26/7/31"
;;   (fmt 1785488645123 "yyyy-MM-dd'T'HH:mm:ssXXX") => "2026-07-31T09:04:05Z"
;;   (fmt 1785488645123 "'week of' d MMMM")         => "week of 31 July"
;; Parsing. java.time's result depends on the type the caller asks for; cljgo
;; always produces an INSTANT, which is the LocalDate/LocalDateTime-at-UTC
;; reading, so the oracle asks for that explicitly:
;;   (.toEpochMilli (.toInstant (.atStartOfDay (LocalDate/parse "31/07/2026" (f "dd/MM/yyyy")) ZoneOffset/UTC)))
;;     => 1785456000000
;;   (.toEpochMilli (.toInstant (LocalDateTime/parse "2026-07-31 09:04:05" (f "yyyy-MM-dd HH:mm:ss")) ZoneOffset/UTC))
;;     => 1785488645000
;;   (.toEpochMilli (.toInstant (OffsetDateTime/parse "2026-07-31T09:04:05+05:30" (f "yyyy-MM-dd'T'HH:mm:ssXXX"))))
;;     => 1785468845000
;;   (.toEpochMilli (.toInstant (LocalDateTime/parse "2026-07-31 09:04 PM" (f "yyyy-MM-dd hh:mm a")) ZoneOffset/UTC))
;;     => 1785531840000
;; The strings and longs below are those JVM values verbatim.
(require '[cljg.date :as date])
[(date/format-pattern 0 "yyyy-MM-dd HH:mm:ss")
 (date/format-pattern 1785488645123 "yyyy-MM-dd HH:mm:ss.SSS")
 (date/format-pattern 1785488645123 "EEE, d MMM yyyy")
 (date/format-pattern 1785488645123 "EEEE d MMMM yyyy")
 (date/format-pattern 1785488645123 "h:mm a")
 (date/format-pattern 1785488645123 "H:m:s")
 (date/format-pattern 1785488645123 "yy/M/d")
 (date/format-pattern 1785488645123 "yyyy-MM-dd'T'HH:mm:ssXXX")
 (date/format-pattern 1785488645123 "'week of' d MMMM")
 (date/parse-pattern "31/07/2026" "dd/MM/yyyy")
 (date/parse-pattern "2026-07-31 09:04:05" "yyyy-MM-dd HH:mm:ss")
 (date/parse-pattern "2026-07-31T09:04:05+05:30" "yyyy-MM-dd'T'HH:mm:ssXXX")
 (date/parse-pattern "2026-07-31 09:04 PM" "yyyy-MM-dd hh:mm a")
 (= 1785488645000 (date/parse-pattern
                    (date/format-pattern 1785488645000 "yyyy-MM-dd HH:mm:ss")
                    "yyyy-MM-dd HH:mm:ss"))
 ;; Unrepresentable tokens are refused, not approximated (decision 4).
 (try (date/format-pattern 0 "yyyy-QQ") (catch Throwable e :threw))
 (try (date/format-pattern 0 "yyyy-DDD") (catch Throwable e :threw))
 (try (date/format-pattern 0 "HH:mm z") (catch Throwable e :threw))
 (try (date/format-pattern 0 "YYYY") (catch Throwable e :threw))
 ;; Malformed runs the JVM also rejects.
 (try (date/format-pattern 0 "yyy") (catch Throwable e :threw))
 (try (date/format-pattern 0 "aa") (catch Throwable e :threw))
 ;; Parsing is stricter than formatting: a pattern that cannot name a whole
 ;; instant is refused rather than silently defaulted.
 (try (date/parse-pattern "07-31" "MM-dd") (catch Throwable e :threw))
 (try (date/parse-pattern "2026-07-31 09:04" "yyyy-MM-dd hh:mm") (catch Throwable e :threw))
 ;; A wrong string must not become a plausible instant.
 (try (date/parse-pattern "2026-02-30" "yyyy-MM-dd") (catch Throwable e :threw))
 (try (date/parse-pattern "Mon 2026-07-31" "EEE yyyy-MM-dd") (catch Throwable e :threw))
 (date/parse-pattern "Fri 2026-07-31" "EEE yyyy-MM-dd")
 ;; `format` still takes a GO layout and is unchanged.
 (date/format 1785488645123 "2006-01-02 15:04:05")]
;; expect: ["1970-01-01 00:00:00" "2026-07-31 09:04:05.123" "Fri, 31 Jul 2026" "Friday 31 July 2026" "9:04 AM" "9:4:5" "26/7/31" "2026-07-31T09:04:05Z" "week of 31 July" 1785456000000 1785488645000 1785468845000 1785531840000 true :threw :threw :threw :threw :threw :threw :threw :threw :threw :threw 1785456000000 "2026-07-31 09:04:05"]
