;; clojure.tools.cli — the GNU-style command-line option parser, ported
;; natively from org.clojure/tools.cli 1.1.230 (spike s52-tools-cli-native,
;; ADR 0097). One vector bundles 15 representative behaviors of parse-opts,
;; summarize, get-default-options and the legacy cli.
;;
;; oracle (real JVM Clojure 1.12.5 + org.clojure/tools.cli 1.1.230, captured
;; via `clojure -Sdeps '{:deps {org.clojure/tools.cli {:mvn/version "1.1.230"}}}'
;; -M <file>` and `(prn …)`), element by element:
;;   1  short+long+required+:default+:parse-fn, one trailing positional arg
;;        => {:options {:port 8080}, :arguments ["extra"],
;;            :summary "  -p, --port PORT  80  Port number", :errors nil}
;;   2  boolean toggle           => {:verbose true}
;;   3  "-vvv" via :update-fn inc => {:verbose 3}
;;   4  --no-color on --[no-]     => {:color false}
;;   5  :validate failure         => ["Failed to validate \"-p 99999\": Must be 0-65535"]
;;   6  unknown option           => ["Unknown option: \"-x\""]
;;   7  missing required arg      => ["Missing required argument for \"--port PORT\""]
;;   8  :missing required option  => ["port is required"]
;;   9  :in-order subcommand stop => ["sub" "-x"]
;;   10 --port=9000 assignment    => {:port 9000}
;;   11 :multi :update-fn conj    => {:file ["a" "b"]}
;;   12 "--" ends option parsing  => ["-p" "x"]
;;   13 summary column layout     => "  -p, --port PORT  80  The port\n  -v, --verbose        Verbose"
;;   14 get-default-options       => {:port 80, :verbose false}
;;   15 legacy cli banner         => [{:port 8080} ["arg"]
;;        "desc\n\n  Switches    Default  Desc\n  --------    -------  ----\n  -p, --port  3000     Port\n"]
;; parse-fn is parse-long (clojure.core, 1.11+) rather than the upstream
;; doc's Integer/parseInt so the same file is byte-identical on JVM and cljgo.
(require '[clojure.tools.cli :as cli])
[(cli/parse-opts ["-p" "8080" "extra"]
                 [["-p" "--port PORT" "Port number" :default 80 :parse-fn parse-long]])
 (:options (cli/parse-opts ["--verbose"] [["-v" "--verbose"]]))
 (:options (cli/parse-opts ["-vvv"] [["-v" nil "v" :id :verbose :default 0 :update-fn inc]]))
 (:options (cli/parse-opts ["--no-color"] [[nil "--[no-]color"]]))
 (:errors (cli/parse-opts ["-p" "99999"]
                          [["-p" "--port PORT" :parse-fn parse-long
                            :validate [#(< 0 % 65536) "Must be 0-65535"]]]))
 (:errors (cli/parse-opts ["-x"] [["-p" "--port PORT"]]))
 (:errors (cli/parse-opts ["--port"] [["-p" "--port PORT"]]))
 (:errors (cli/parse-opts [] [["-p" "--port PORT" :missing "port is required"]]))
 (:arguments (cli/parse-opts ["-v" "sub" "-x"] [["-v" "--verbose"]] :in-order true))
 (:options (cli/parse-opts ["--port=9000"] [["-p" "--port PORT" :parse-fn parse-long]]))
 (:options (cli/parse-opts ["-f" "a" "-f" "b"]
                           [["-f" "--file NAME" :default [] :update-fn conj :multi true]]))
 (:arguments (cli/parse-opts ["--" "-p" "x"] [["-p" "--port PORT"]]))
 (:summary (cli/parse-opts [] [["-p" "--port PORT" "The port" :default 80]
                               ["-v" "--verbose" "Verbose"]]))
 (cli/get-default-options [["-p" "--port PORT" :default 80] ["-v" "--verbose" :default false]])
 (cli/cli ["-p" "8080" "arg"] "desc" ["-p" "--port" "Port" :default 3000 :parse-fn parse-long])]
;; expect: [{:options {:port 8080}, :arguments ["extra"], :summary "  -p, --port PORT  80  Port number", :errors nil} {:verbose true} {:verbose 3} {:color false} ["Failed to validate \"-p 99999\": Must be 0-65535"] ["Unknown option: \"-x\""] ["Missing required argument for \"--port PORT\""] ["port is required"] ["sub" "-x"] {:port 9000} {:file ["a" "b"]} ["-p" "x"] "  -p, --port PORT  80  The port\n  -v, --verbose        Verbose" {:port 80, :verbose false} [{:port 8080} ["arg"] "desc\n\n  Switches    Default  Desc\n  --------    -------  ----\n  -p, --port  3000     Port\n"]]
