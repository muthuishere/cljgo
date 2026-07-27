package core

import _ "embed"

// DataJSONSource is core/data_json.cljg — clojure.data.json (ADR 0097): the
// org.clojure/data.json contrib codec brought in natively, the exact
// read-str/read/write-str/write API over a Go host codec (pkg/bri/cljson).
// It is NOT a boot source — it rides bri.Specs() as a lazy, opt-in namespace
// (loaded on first require in the interpreter, opt-in-linked in AOT), so a
// binary that never requires it pays zero bytes.
//
//go:embed data_json.cljg
var DataJSONSource string
