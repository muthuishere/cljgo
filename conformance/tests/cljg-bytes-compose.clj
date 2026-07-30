;; ONE byte representation, and byte producers COMPOSE with byte consumers
;; (ADR 0110 asks 1/5, the follow-up fix). Two things are frozen here:
;;
;;   1. EVERY route to bytes agrees. cljg.io/read-bytes, cljg.stream/read-bytes,
;;      cljg.stream/chunks, cljg.compress/gzip+gunzip and
;;      cljg.security/base64-decode-bytes all hand back the SAME signed
;;      byte-array, so reading one file two ways cannot disagree in SIGN
;;      (0xFF is -1 on every route, never 255).
;;   2. EVERY byte consumer accepts what a byte producer emits. If (bytes? x)
;;      is true, x may be gunzipped, hashed, hexed, base64'd, written to a file
;;      and written to a stream. The motivating case (an MCP resource blob) is
;;      the write -> read -> gunzip and base64-decode-bytes -> gunzip chains
;;      below: before this, both threw on a value that answered `bytes?` true.
;;
;; oracle: partial. cljg.io / cljg.stream / cljg.compress / cljg.security are
;; cljgo host namespaces with no JVM package, so this file cannot run through
;; the `clojure` CLI verbatim. The SIGNEDNESS that everything here hangs on was
;; re-derived against it at authoring time (clojure 1.12.5, 2026-07-30):
;;   (vec (byte-array [104 101 -1 0]))            => [104 101 -1 0]
;;   (java.nio.file.Files/write p (byte-array [-1 0 65]) …)
;;   (vec (java.nio.file.Files/readAllBytes p))   => [-1 0 65]
;;   (class (java.nio.file.Files/readAllBytes p)) => [B   ; signed byte[]
;; i.e. a 0xFF byte reads back as -1, not 255 — so signed is the ONE
;; representation. The rest is cljgo's own surface, frozen against cljgo's
;; output; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljg.io :as io])
(require '[cljg.stream :as st])
(require '[cljg.compress :as z])
(require '[cljg.security :as sec])
(let [p    (io/temp-file "cljg-compose-" ".bin")
      gzp  (io/temp-file "cljg-compose-" ".gz")
      _    (io/write-bytes p (byte-array [-1 0 65]))
      _    (io/write-bytes gzp (z/gzip "composed payload"))
      via-io     (io/read-bytes p)
      ;; read-bytes/chunks read ONE chunk — they are not terminal, so neither
      ;; drains to EOF and neither releases the file on its own. That is the
      ;; API working as documented, and it is exactly what with-open is for.
      ;; Without it the handle stays open and (io/delete! p) below fails on
      ;; Windows ("the process cannot access the file"), while POSIX unlinks an
      ;; open file happily — the divergence CI caught on b8b7718/7f287e9.
      via-stream (with-open [s (st/of-file p)] (st/read-bytes s))
      via-chunk  (with-open [s (st/of-file p)] (first (st/chunks s)))
      out
      [;; --- 1. one representation, whatever the route ---
       (vec via-io)
       (vec via-stream)
       (vec via-chunk)
       (= (vec via-io) (vec via-stream) (vec via-chunk))
       [(bytes? via-io) (bytes? via-stream) (bytes? via-chunk)]
       ;; a compressed blob is bytes on the same terms — gzip's magic number
       ;; 0x1f 0x8b reads as 31 -117, SIGNED, not 31 139.
       (vec (take 2 (z/gzip "x")))
       (bytes? (z/gzip "x"))
       (vec (z/gunzip (z/gzip (byte-array [-1 0 65]))))

       ;; --- 2. producers compose with consumers ---
       ;; write -> read -> gunzip: the file round-trip the ADR is FOR.
       (z/gunzip (io/read-bytes gzp) {:as :string})
       ;; base64-decode-bytes -> gunzip: what its docstring advertises.
       (z/gunzip (sec/base64-decode-bytes (sec/base64-encode (z/gzip "b64 blob"))) {:as :string})
       ;; a stream chunk is gunzippable too.
       (with-open [s (st/of-file gzp)] (z/gunzip (st/read-bytes s) {:as :string}))
       ;; digests + codecs over read bytes, no lossy string detour.
       (sec/sha256 (io/read-bytes p))
       (sec/hex (io/read-bytes p))
       (sec/base64-encode (io/read-bytes p))
       (sec/hmac "k" (io/read-bytes p))
       ;; and back out to a file / a stream.
       (let [q (io/temp-file "cljg-compose-" ".out")]
         (io/write-bytes q (io/read-bytes p))
         (let [n (vec (io/read-bytes q))]
           (io/delete! q)
           n))
       (let [q (io/temp-file "cljg-compose-" ".out")]
         (with-open [w (st/to-file q)]
           (st/write w (io/read-bytes p)))
         (let [n (vec (io/read-bytes q))]
           (io/delete! q)
           n))

       ;; --- 3. of-file is with-open-closeable, as its docstring says ---
       (with-open [s (st/of-file p)] (count (st/read-all s)))

       ;; --- 4. a non-map opts is REJECTED, never silently truncating ---
       (try (io/write-bytes p (byte-array [1]) true)
            (catch Throwable e (.getMessage e)))
       (try (st/to-file p true)
            (catch Throwable e (.getMessage e)))
       ;; the map form still appends, and nil still truncates.
       (do (io/write-bytes p (byte-array [-1 0 65]))
           (io/write-bytes p (byte-array [66]) {:append true})
           (vec (io/read-bytes p)))]]
  (io/delete! p)
  (io/delete! gzp)
  out)
;; expect: [[-1 0 65] [-1 0 65] [-1 0 65] true [true true true] [31 -117] true [-1 0 65] "composed payload" "b64 blob" "composed payload" "0fa3e62511779f0398b77cad37b3cc4763bb96253b91fcd61500f8a979ad9920" "ff0041" "/wBB" "4fe5f27186ac82d9dd987dc2c49672ee92e16d63b98ac1027a7c71c1e7ecc12e" [-1 0 65] [-1 0 65] 3 "cljg.io/write-bytes: options must be a map (expects {:append true}, found: true)" "cljg.stream/to-file: options must be a map (expects {:append true}, found: true)" [-1 0 65 66]]
