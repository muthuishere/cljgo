;; cljg.socket (ADR 0103, spike s59): loopback TCP echo round-trip. listen on
;; :port 0 (OS-assigned — the handle's :port is the REAL bound port, so it
;; composes with dial), accept in a future, echo one line back through the
;; connection's :out, and read it on the dialing side. The connection's :in /
;; :out are cljg.stream handles, so the whole read/write surface is st/* —
;; sockets compose with the ONE stream abstraction. Output prints only the
;; payload (never the ephemeral port), so it is deterministic.
;;
;; oracle: skip — cljg.socket is a cljgo host namespace (Bun.listen analog)
;; with no JVM package, so this file cannot run through the `clojure` CLI.
;; Frozen against cljgo's own output (oracle: n/a — cljgo-native);
;; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljg.socket :as sock] '[cljg.stream :as st])
(let [l      (sock/listen {:port 0})
      server (future
               (let [c (sock/accept l)]
                 (st/write-line (:out c) (st/read-line (:in c)))
                 (sock/close c)))
      c      (sock/dial {:host "127.0.0.1" :port (:port l)})]
  (st/write-line (:out c) "hello over tcp")
  (let [got (st/read-line (:in c))]
    @server
    (sock/close c)
    (sock/close l)
    [(pos? (:port l)) got]))
;; expect: [true "hello over tcp"]
