// S21 spike: prove `ffi/deflib`'s REPL-live registration path is buildable
// on purego — not just "purego can call C" (S7 already proved that with
// static Go func vars) but "a runtime declaration with no compile-time Go
// signature can become a callable purego binding". See deflib.go for the
// mechanism, VERDICT.md for the verdict.
//
// Run: cd spikes/s21-c-ffi-purego && CGO_ENABLED=0 go run .
package main

import (
	"fmt"
	"os"
	"runtime"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

func demoNoArgAndIntArg() {
	fmt.Println("\n== 1. no-arg -> int, int-arg -> double (libSystem, libm) ==")

	sys, err := Declare("posix", darwinLibPath("libSystem.B.dylib", "libc.so.6"), []FnDecl{
		{CljName: "getpid", CSymbol: "getpid", Args: nil, Ret: KInt32},
	})
	must(err)
	pid, err := sys.Fns["getpid"].Call()
	must(err)
	fmt.Printf("(posix/getpid) => %v   (os pid, sanity: nonzero)\n", pid)

	m, err := Declare("libm", darwinLibPath("libm.dylib", "libm.so.6"), []FnDecl{
		{CljName: "cos", CSymbol: "cos", Args: []Kind{KFloat64}, Ret: KFloat64},
		{CljName: "sqrt", CSymbol: "sqrt", Args: []Kind{KFloat64}, Ret: KFloat64},
	})
	must(err)
	cosv, err := m.Fns["cos"].Call(0.0)
	must(err)
	sqrtv, err := m.Fns["sqrt"].Call(2.0)
	must(err)
	fmt.Printf("(libm/cos 0.0) => %v   (libm/sqrt 2.0) => %v\n", cosv, sqrtv)
}

func demoBufferOutParam() {
	fmt.Println("\n== 2. pointer/buffer arg (libz crc32 over a Go byte slice) ==")
	z, err := Declare("libz", darwinLibPath("libz.dylib", "libz.so.1"), []FnDecl{
		// uLong crc32(uLong crc, const Bytef *buf, uInt len)
		// Bytef* has no clean reflect-Kind of its own in this prototype's
		// vocabulary (design/05's :ptr) — it is passed as a raw uintptr,
		// exactly the pattern S7 verified for sqlite3's out-params. The
		// slice->uintptr conversion below is what deflib's :ptr marshaling
		// helper would do on the caller's behalf.
		{CljName: "crc32", CSymbol: "crc32", Args: []Kind{KInt64, KUintptr, KInt32}, Ret: KInt64},
	})
	must(err)
	buf := []byte("cljgo")
	ptr := uintptr(unsafe.Pointer(&buf[0]))
	crc, err := z.Fns["crc32"].Call(int64(0), ptr, int32(len(buf)))
	must(err)
	// KeepAlive: the Go slice must not be collected/moved while C holds ptr.
	runtimeKeepAlive(buf)
	fmt.Printf("(libz/crc32 0 %q %d) => %v  (expect a stable nonzero CRC)\n", buf, len(buf), crc)
}

func demoFailureMissingLib() {
	fmt.Println("\n== 3a. failure: missing library (declaration-time, not first-call) ==")
	_, err := Declare("nope", "libDefinitelyDoesNotExist9000.dylib", []FnDecl{
		{CljName: "whatever", CSymbol: "whatever", Args: nil, Ret: KVoid},
	})
	fmt.Printf("(ffi/deflib nope \"libDefinitelyDoesNotExist9000.dylib\" ...) => error: %v\n", err)
}

func demoFailureMissingSymbol() {
	fmt.Println("\n== 3b. failure: symbol not in the library (declaration-time) ==")
	_, err := Declare("libm2", darwinLibPath("libm.dylib", "libm.so.6"), []FnDecl{
		{CljName: "not-a-real-fn", CSymbol: "this_symbol_does_not_exist_in_libm", Args: nil, Ret: KVoid},
	})
	fmt.Printf("(ffi/deflib libm2 ... (not-a-real-fn \"this_symbol_does_not_exist_in_libm\" ...)) => error: %v\n", err)
}

func demoFailureWrongArity() {
	fmt.Println("\n== 3c. failure: wrong arity at call time (caught before reflect.Call) ==")
	m, err := Declare("libm3", darwinLibPath("libm.dylib", "libm.so.6"), []FnDecl{
		{CljName: "cos", CSymbol: "cos", Args: []Kind{KFloat64}, Ret: KFloat64},
	})
	must(err)
	_, err = m.Fns["cos"].Call(1.0, 2.0) // declared 1 arg, called with 2
	fmt.Printf("(libm3/cos 1.0 2.0) => error: %v\n", err)
}

func demoFailureVariadicRejected() {
	fmt.Println("\n== 3d. failure: variadic C fn rejected at deflib expansion time ==")
	_, err := Declare("libsys2", darwinLibPath("libSystem.B.dylib", "libc.so.6"), []FnDecl{
		{CljName: "printf", CSymbol: "printf", Args: []Kind{KString}, Ret: KInt32, Variadic: true},
	})
	fmt.Printf("(ffi/deflib libsys2 ... (printf \"printf\" [:string] :int :variadic true)) => error: %v\n", err)
}

func demoWrongSignatureCorruption() {
	fmt.Println("\n== 3e. failure: WRONG signature (not a missing symbol) — corruption, not a crash ==")
	fmt.Println("  (documented from S7, re-affirmed here: declaring cos as (:int32)->:int32 instead")
	fmt.Println("   of (:float64)->:float64 does not error — libm's cos passes/returns its double in the")
	fmt.Println("   float ABI register class, purego reads the int ABI register class instead. No panic,")
	fmt.Println("   no error, just a wrong number (real cos(0)=1, this reads back as 0).")
	fmt.Println("   ⇒ deflib cannot make this class of mistake safe; it can only make it LOUD in docs.")
	m, err := Declare("libm4", darwinLibPath("libm.dylib", "libm.so.6"), []FnDecl{
		{CljName: "cos_mis_typed", CSymbol: "cos", Args: []Kind{KInt32}, Ret: KInt32}, // WRONG: cos takes/returns double
	})
	must(err)
	v, err := m.Fns["cos_mis_typed"].Call(int32(0))
	fmt.Printf("  (libm4/cos_mis_typed 0) => %v, err=%v   <- silently wrong, not zero-cos(0)=1\n", v, err)
}

func runtimeKeepAlive(b []byte) { _ = b } // stands in for runtime.KeepAlive(b) for clarity in the demo

func darwinLibPath(mac, linux string) string {
	if fileExists("/usr/lib/" + mac) {
		return "/usr/lib/" + mac
	}
	return mac // dyld shared cache: bare name resolves even when the file isn't on disk (S7 finding)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func buildInfo() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	fmt.Println("S21 — ffi/deflib dynamic registration prototype")
	fmt.Println("purego version: (see go.mod) github.com/ebitengine/purego v0.10.1")
	fmt.Println("GOOS/GOARCH:", buildInfo())

	demoNoArgAndIntArg()
	demoBufferOutParam()
	demoFailureMissingLib()
	demoFailureMissingSymbol()
	demoFailureWrongArity()
	demoFailureVariadicRejected()
	demoWrongSignatureCorruption()

	fmt.Println("\n== 4. REPL-liveness: register, call, RE-declare with a different symbol, call again ==")
	demoRedeclareLive()

	fmt.Println("\nDone.")
	_ = time.Now
	_ = purego.RTLD_NOW
}
