package format14

import "fmt"

// FormatError mirrors the SIMPLE CLASS NAME of the java.util.* exception
// java.util.Formatter would throw, so it can be compared 1:1 against the
// oracle's captured ExClass without re-deriving Java's class hierarchy.
type FormatError struct {
	Class string
	Msg   string
}

func (e *FormatError) Error() string { return e.Class + ": " + e.Msg }

func errUnknownConversion(conv byte) error {
	return &FormatError{"UnknownFormatConversionException", fmt.Sprintf("Conversion = '%c'", conv)}
}

func errIllegalConversion(conv byte, typeName string) error {
	return &FormatError{"IllegalFormatConversionException", fmt.Sprintf("%c != %s", conv, typeName)}
}

func errMissingArg(conv byte) error {
	return &FormatError{"MissingFormatArgumentException", fmt.Sprintf("Format specifier '%%%c'", conv)}
}

func errDuplicateFlags(flags string) error {
	return &FormatError{"DuplicateFormatFlagsException", "Flags = '" + flags + "'"}
}

func errIllegalFlags(flags string) error {
	return &FormatError{"IllegalFormatFlagsException", "Flags = '" + flags + "'"}
}
