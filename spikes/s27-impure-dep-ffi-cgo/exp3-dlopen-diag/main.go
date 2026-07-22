// S27 exp3 — what a purego Dlopen/Dlsym FAILURE actually looks like, for
// real, on this host. ADR 0015 wants structured diagnostics; ADR 0044 §3
// claims "named, positioned errors ... never a raw panic". This measures
// which of dlopen's failure modes return an error and which panic.
package main

import (
	"fmt"
	"runtime"

	"github.com/ebitengine/purego"
)

// try runs f, converting any panic into a reported string, so a panic and
// an error are distinguishable in the output rather than killing the run.
func try(label string, f func() (string, error)) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%-34s PANIC: %v\n", label, r)
		}
	}()
	s, err := f()
	if err != nil {
		fmt.Printf("%-34s error: %v\n", label, err)
		return
	}
	fmt.Printf("%-34s ok: %s\n", label, s)
}

func main() {
	fmt.Printf("host: %s/%s\n\n", runtime.GOOS, runtime.GOARCH)

	// 1. Library that does not exist anywhere.
	try("absent library", func() (string, error) {
		h, err := purego.Dlopen("libtotally_absent_s27.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		return fmt.Sprint(h), err
	})

	// 2. WRONG-OS soname: the Linux name for a library that IS present here
	//    under a different extension. This is the platform-matrix trap — a
	//    dependency hardcoding "libsqlite3.so" is broken on darwin.
	try("wrong-OS name (libsqlite3.so)", func() (string, error) {
		h, err := purego.Dlopen("libsqlite3.so", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		return fmt.Sprint(h), err
	})

	// 3. The correct per-OS name, to prove the library really is here.
	var handle uintptr
	try("correct name (libsqlite3.dylib)", func() (string, error) {
		h, err := purego.Dlopen("libsqlite3.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		handle = h
		return fmt.Sprintf("handle=%#x", h), err
	})

	// 4. Symbol that does not exist in a library that does.
	try("absent symbol in present lib", func() (string, error) {
		p, err := purego.Dlsym(handle, "sqlite3_no_such_symbol_s27")
		return fmt.Sprintf("%#x", p), err
	})

	// 5. RegisterLibFunc against an absent symbol — the ffi/deflib path,
	//    where purego is documented to panic rather than return.
	try("RegisterLibFunc absent symbol", func() (string, error) {
		var fn func() int32
		purego.RegisterLibFunc(&fn, handle, "sqlite3_no_such_symbol_s27")
		return "registered (no error!)", nil
	})

	// 6. RegisterLibFunc against an absent LIBRARY handle (0).
	try("RegisterLibFunc on nil handle", func() (string, error) {
		var fn func() int32
		purego.RegisterLibFunc(&fn, 0, "sqlite3_libversion_number")
		return "registered (no error!)", nil
	})

	// 7. The happy path, to show a working call for contrast.
	try("working call", func() (string, error) {
		var libversion func() int32
		purego.RegisterLibFunc(&libversion, handle, "sqlite3_libversion_number")
		return fmt.Sprintf("sqlite3_libversion_number()=%d", libversion()), nil
	})
}
