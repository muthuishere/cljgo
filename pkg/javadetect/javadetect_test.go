package javadetect

import (
	"bufio"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

func read(t *testing.T, src string, opts ...reader.Option) []any {
	t.Helper()
	r := reader.New(bufio.NewReader(strings.NewReader(src)),
		append([]reader.Option{reader.WithFilename("t.clj")}, opts...)...)
	forms, err := r.ReadAll()
	if err != nil {
		t.Fatalf("read %q: %v", src, err)
	}
	return forms
}

// TestConsumeJavaFlagsTheSelfIdentifyingSurfaces covers what the consume-side
// gate adds on top of the publish-side classifier: the ns-form clauses and the
// class-defining special forms a jar's JVM namespaces announce themselves with.
func TestConsumeJavaFlagsTheSelfIdentifyingSurfaces(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"ns :import", `(ns x (:import [java.io File]))`, ":import"},
		{"ns :import, no vector", `(ns x (:import java.net.URI))`, ":import"},
		{"ns :gen-class", `(ns x (:gen-class))`, ":gen-class"},
		{"gen-class form", `(gen-class :name "X")`, "gen-class"},
		{"definterface", `(definterface IFoo (bar []))`, "definterface"},
		{"proxy", `(defn p [] (proxy [Object] [] (toString [] "x")))`, "proxy"},
		{"reify on a java type", `(reify java.lang.Runnable (run [_] nil))`, "reify"},
		{"import special form", `(import 'java.util.Date)`, "import"},
		{"java package call", `(java.util.UUID/randomUUID)`, "java.util.UUID"},
		{"Class/member static call", `(System/getProperty "x")`, "System/getProperty"},
		{"clojure.java.io", `(clojure.java.io/file "x")`, "clojure.java."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := ConsumeJava(read(t, tc.src))
			if len(hits) == 0 {
				t.Fatalf("%q was not flagged", tc.src)
			}
			joined := ""
			for _, h := range hits {
				joined += h.Detail + "\n"
			}
			if !strings.Contains(joined, tc.want) {
				t.Errorf("%q: want a hit mentioning %q, got:\n%s", tc.src, tc.want, joined)
			}
		})
	}
}

// TestConsumeJavaIsZeroFalsePositive is the load-bearing half: the classifier
// is CERTAIN-only. Flagging any of these would reject correct Go-hosted code.
func TestConsumeJavaIsZeroFalsePositive(t *testing.T) {
	pure := []string{
		`(defn f [o] (.method o))`,                   // undecidable, Go-valid
		`(defn f [o] (.-field o))`,                   //
		`(instance? String x)`,                       // class-ref VALUE
		`(try (f) (catch Exception e nil))`,          // class-ref VALUE
		`(def c String)`,                             // class-ref VALUE
		`(ns x (:require [clojure.string :as str]))`, // an ordinary ns form
		`(ns x (:refer-clojure :exclude [update]))`,  //
		`(defrecord R [a b])`,                        // pure Clojure
		`(defprotocol P (m [this]))`,                 //
		`(reify P (m [_] 1))`,                        // reify on a PROTOCOL
		`(extend-type String P (m [_] 1))`,           // tabled class, but…
		`(defn javafoo [] 1)`,                        // "java" prefix, not the package
		`(def m {:import 1 :gen-class 2})`,           // keywords as DATA
	}
	for _, src := range pure {
		if hits := ConsumeJava(read(t, src)); len(hits) > 0 {
			// extend-type on a tabled bare class IS a deliberate flag; assert
			// the rest are clean and let that one through explicitly.
			if strings.HasPrefix(src, "(extend-type String") {
				continue
			}
			t.Errorf("FALSE POSITIVE on %q: %+v", src, hits)
		}
	}
}

// TestClassifierRunsAfterReaderConditionals is the medley guarantee: a Java
// form fenced inside #?(:clj …) is not in the forms at all, so it cannot be
// flagged. This is what makes the reader-output classifier strictly stronger
// than the text scan spike s50 asked for.
func TestClassifierRunsAfterReaderConditionals(t *testing.T) {
	src := `(ns medley.core)
(defn now [] #?(:clj (java.util.Date.) :default :now))
(defn u [] #?(:clj (java.util.UUID/randomUUID) :default "u"))
(ns other (:import #?(:clj [java.util Date])))`
	if hits := ConsumeJava(read(t, src)); len(hits) > 0 {
		t.Fatalf("fenced Java was flagged — a text scan's failure mode: %+v", hits)
	}
	// The same source WITH the JVM's feature supplied does flag, proving the
	// forms (not the text) are what is being classified.
	forms := read(t, src, reader.WithFeatures(lang.NewKeyword("clj")))
	if hits := ConsumeJava(forms); len(hits) == 0 {
		t.Fatal("with :clj supplied the same source must flag")
	}
}

// TestPublishSurfaceIsUnchanged pins that the consume-side additions did NOT
// leak into the publish-side courtesy diagnostic.
func TestPublishSurfaceIsUnchanged(t *testing.T) {
	if hits := CertainJava(read(t, `(ns x (:import [java.io File]))`)); len(hits) != 0 {
		t.Errorf("CertainJava must not gain the ns-clause surface: %+v", hits)
	}
	if hits := ConsumeJava(read(t, `(ns x (:import [java.io File]))`)); len(hits) == 0 {
		t.Error("ConsumeJava must have the ns-clause surface")
	}
}
