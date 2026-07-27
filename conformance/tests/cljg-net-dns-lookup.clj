;; cljg.net.dns (ADR 0103 wave 1, spike s60): `lookup` resolves via the
;; pure-Go stdlib resolver (net.Resolver{PreferGo:true}) — "localhost" comes
;; from the hosts file, so this stays SELF-CONTAINED (loopback only, no
;; external network); the result is a sorted vector of IP strings containing a
;; loopback address (normalized here to a boolean — v4/v6 presence varies by
;; hosts file, never frozen as the whole set). A guaranteed-nonexistent name
;; under the RFC 6761 reserved .invalid TLD throws an ex-info naming the host;
;; its ex-data SHAPE ({:type :query :host}) is frozen — the message wraps a
;; platform-dependent resolver error, so the shape, not the text, is pinned —
;; and the error path runs twice to prove the shape is deterministic.
;;
;; oracle: n/a — cljgo-native (cljg.net.dns is a cljgo host namespace with no
;; JVM package). Frozen against cljgo's output; REPL-vs-binary parity is
;; enforced by the dual harness.
(require '[cljg.net.dns :as dns])
(let [ips (dns/lookup "localhost")
      shape (fn []
              (try
                (dns/lookup "no-such-host.invalid")
                :unexpected-success
                (catch Exception e
                  (select-keys (ex-data e) [:type :query :host]))))
      s1 (shape)
      s2 (shape)]
  [(vector? ips)
   (boolean (some #{"127.0.0.1" "::1"} ips))
   (= s1 s2)
   s1])
;; expect: [true true true {:type :cljg.net.dns/error, :query :lookup, :host "no-such-host.invalid"}]
