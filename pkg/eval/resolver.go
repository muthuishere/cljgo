package eval

import (
	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// ReaderResolver returns the reader.Resolver backed by the current
// namespace (*ns*). The REPL driver and file loads inject it into
// their readers. Since ADR 0043 the resolver itself is stateless and
// lives in pkg/corelib (read-string needs it without an Evaluator);
// this method survives as the seam the drivers already use.
func (e *Evaluator) ReaderResolver() reader.Resolver { return corelib.NSResolver() }
