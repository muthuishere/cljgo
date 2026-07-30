;; cljg.socket (ADR 0103, spike s59): loopback UDP datagram round-trip. UDP is
;; the separate DATAGRAM surface (a packet socket is not a byte stream): two
;; sockets on ephemeral loopback ports, client sends one datagram to the
;; server, the server reads it (udp-recv blocks for one packet and reports the
;; sender), replies to the sender's port, and the client reads the reply.
;; Output prints only the payloads (never the ephemeral ports), so it is
;; deterministic; timeouts bound the waits so a lost packet fails fast instead
;; of hanging the harness (loopback UDP does not drop in practice).
;;
;; oracle: skip — cljg.socket is a cljgo host namespace (Bun.listen analog)
;; with no JVM package, so this file cannot run through the `clojure` CLI.
;; Frozen against cljgo's own output (oracle: n/a — cljgo-native);
;; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljg.socket :as sock])
(let [srv (sock/udp-listen {:port 0})
      cli (sock/udp-listen {:port 0})]
  (sock/udp-send cli "127.0.0.1" (:port srv) "hello over udp")
  (let [req   (sock/udp-recv srv {:timeout-ms 5000})
        _     (sock/udp-send srv (:host req) (:port req) (str (:data req) "!"))
        reply (sock/udp-recv cli {:timeout-ms 5000})]
    (sock/close srv)
    (sock/close cli)
    [(:data req) (:data reply)]))
;; expect: ["hello over udp" "hello over udp!"]
