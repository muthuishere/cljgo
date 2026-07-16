package format14

// FormatDirect runs the corpus's (fmt, args) pair through the direct
// interpreter candidate (B).
func FormatDirect(f string, args []any) (string, error) {
	return Format(DirectRender, f, args)
}

// FormatTranslate runs it through the translate-then-delegate candidate (A).
func FormatTranslate(f string, args []any) (string, error) {
	return Format(TranslateRender, f, args)
}
