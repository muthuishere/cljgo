;; clojure.string capitalize/upper-case/lower-case/starts-with?/ends-with?
;; (batch/error-files): these functions call `.toString()` on a
;; CharSequence-hinted position, which is NOT the same as `str` — nil throws
;; (real NPE), but any other value (keyword, symbol, number) coerces via its
;; own toString (a keyword's toString keeps the leading `:`, unlike `str`
;; which doesn't add one either but a NIL toString differs: (str nil) => "",
;; (.toString nil) throws). starts-with?/ends-with?'s SECOND arg (substr) is
;; different: it's typed java.lang.String outright, so only a literal
;; string coerces there — a keyword/symbol in that position throws a
;; ClassCastException, same as real Clojure.
;; oracle (clojure 1.12.5): [(str/capitalize :asDf/aSdf) (str/capitalize 1)
;; (str/capitalize 'asDf/aSdf) (str/starts-with? 'ab "a")
;; (str/starts-with? :ab ":a")] => [":asdf/asdf" "1" "Asdf/asdf" true true];
;; (str/capitalize nil) throws; (str/starts-with? "ab" :a) throws.
[[(clojure.string/capitalize :asDf/aSdf) (clojure.string/capitalize 1)
  (clojure.string/capitalize 'asDf/aSdf)
  (clojure.string/starts-with? 'ab "a")
  (clojure.string/starts-with? :ab ":a")]
 (try (clojure.string/capitalize nil) :nothrow (catch Exception _e :threw))
 (try (clojure.string/starts-with? "ab" :a) :nothrow (catch Exception _e :threw))]
;; expect: [[":asdf/asdf" "1" "Asdf/asdf" true true] :threw :threw]
