package publish

// The ADR 0054 decision-4 `certain-java?` COURTESY diagnostic. The classifier
// itself now lives in pkg/javadetect, shared with the ADR 0095 consume-side
// gate so the two directions of one decision can never drift. Behaviour here
// is unchanged: publish calls the publish-side surface (CertainJava), not the
// consume-side superset (javadetect.ConsumeJava).

import "github.com/muthuishere/cljgo/pkg/javadetect"

// Diag is one certain-Java finding with the position a courtesy message cites.
type Diag = javadetect.Diag

// CertainJava scans reader forms for the self-identifying JVM surfaces only
// and returns a diagnostic per certain-Java form, in source order. It is
// certain-only and zero-FP; it is a diagnostic, never a gate.
func CertainJava(forms []any) []Diag { return javadetect.CertainJava(forms) }

// CertainJavaFile reads path with pkg/reader and runs CertainJava, tagging
// each Diag's File with path.
func CertainJavaFile(path string) ([]Diag, error) { return javadetect.CertainJavaFile(path) }
