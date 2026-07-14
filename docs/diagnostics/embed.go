// Package diagnostics embeds the diagnostic explain pages and the
// append-only registry lock snapshot so pkg/diag can serve
// Explain(code) from the binary (ADR 0015; design.md D2).
//
// One <CODE>.md page per registered code in pkg/diag/registry.go;
// registry.lock is the committed code+title snapshot that makes the
// registry append-only (enforced by pkg/diag tests).
package diagnostics

import "embed"

// FS holds every explain page and the registry lock.
//
//go:embed *.md registry.lock
var FS embed.FS
