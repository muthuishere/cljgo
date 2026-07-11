// S7 spike: prove the C-ecosystem story without cgo, via purego dlopen/dlsym.
// Target: system sqlite3 on macOS (dyld-shared-cache resolved, no file on disk).
//
// Patterns exercised (each marked PATTERN below):
//  1. C char* return           -> Go `string` return (purego copies into Go memory)
//  2. Go string arg            -> C char* (purego appends NUL + copies per call)
//  3. Pointer-to-pointer out   -> *uintptr (sqlite3**, sqlite3_stmt**, char**)
//  4. Opaque handle            -> uintptr passed by value
//  5. C callback               -> purego.NewCallback (sqlite3_exec's row callback)
//  6. char** array in callback -> unsafe pointer arithmetic + manual C-string read
//  7. C int return codes       -> int32 (C int is 32-bit; Go int would work on
//     arm64 but int32 states intent)
//  8. Variadic C fn            -> demonstrated FAILING (sqlite3_mprintf)
package main

import (
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	sqliteOK   = 0
	sqliteRow  = 100
	sqliteDone = 101
)

// goString reads a NUL-terminated C string at p into a Go string.
// Needed where purego's automatic `string` return conversion can't be used
// (inside callbacks, or for char** elements). purego does not export one.
func goString(p uintptr) string {
	if p == 0 {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Pointer(p + uintptr(n))) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(unsafe.Pointer(p)), n))
}

// deref reads the i-th pointer of a C pointer array (char** etc).
func deref(arr uintptr, i int) uintptr {
	return *(*uintptr)(unsafe.Pointer(arr + uintptr(i)*unsafe.Sizeof(uintptr(0))))
}

func main() {
	// ---- dlopen the system sqlite ------------------------------------------
	// /usr/lib/libsqlite3.dylib does NOT exist on disk on modern macOS (it
	// lives in the dyld shared cache), but dlopen() resolves cache paths fine.
	lib, err := purego.Dlopen("/usr/lib/libsqlite3.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		// Fallback: bare name lets dyld search its default paths + cache.
		lib, err = purego.Dlopen("libsqlite3.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	}
	if err != nil {
		panic(fmt.Sprintf("dlopen libsqlite3: %v", err))
	}
	fmt.Printf("dlopen ok, handle=%#x (file absent on disk; dyld shared cache resolved it)\n", lib)

	// ---- bind functions ----------------------------------------------------
	// PATTERN 1: const char *sqlite3_libversion(void) -> `func() string`
	var libversion func() string
	purego.RegisterLibFunc(&libversion, lib, "sqlite3_libversion")

	// PATTERN 3: int sqlite3_open(const char *filename, sqlite3 **ppDb)
	// sqlite3** becomes *uintptr: C writes the new handle through the pointer.
	var open func(path string, ppDb *uintptr) int32
	purego.RegisterLibFunc(&open, lib, "sqlite3_open")

	// PATTERN 5: int sqlite3_exec(sqlite3*, const char *sql,
	//        int (*cb)(void*,int,char**,char**), void *arg, char **errmsg)
	// The callback slot is declared uintptr so we can pass 0 (NULL) or a
	// purego.NewCallback value.
	var exec func(db uintptr, sql string, cb uintptr, arg uintptr, errmsg *uintptr) int32
	purego.RegisterLibFunc(&exec, lib, "sqlite3_exec")

	// int sqlite3_prepare_v2(sqlite3*, const char *sql, int nByte,
	//                        sqlite3_stmt **ppStmt, const char **pzTail)
	var prepareV2 func(db uintptr, sql string, nByte int32, ppStmt *uintptr, pzTail *uintptr) int32
	purego.RegisterLibFunc(&prepareV2, lib, "sqlite3_prepare_v2")

	var step func(stmt uintptr) int32
	purego.RegisterLibFunc(&step, lib, "sqlite3_step")

	// PATTERN 1 again: const unsigned char *sqlite3_column_text(stmt, int col)
	var columnText func(stmt uintptr, col int32) string
	purego.RegisterLibFunc(&columnText, lib, "sqlite3_column_text")

	var columnInt func(stmt uintptr, col int32) int32
	purego.RegisterLibFunc(&columnInt, lib, "sqlite3_column_int")

	var finalize func(stmt uintptr) int32
	purego.RegisterLibFunc(&finalize, lib, "sqlite3_finalize")

	var closeDB func(db uintptr) int32
	purego.RegisterLibFunc(&closeDB, lib, "sqlite3_close")

	var errmsgFn func(db uintptr) string
	purego.RegisterLibFunc(&errmsgFn, lib, "sqlite3_errmsg")

	var freeFn func(p uintptr)
	purego.RegisterLibFunc(&freeFn, lib, "sqlite3_free")

	// ---- 1. version --------------------------------------------------------
	fmt.Printf("sqlite3_libversion() = %q\n", libversion())

	// ---- 2. open :memory: (pointer-to-pointer out-param) -------------------
	var db uintptr // PATTERN 3+4: C fills this with the sqlite3* handle
	if rc := open(":memory:", &db); rc != sqliteOK {
		panic(fmt.Sprintf("sqlite3_open rc=%d", rc))
	}
	fmt.Printf("sqlite3_open(\":memory:\") ok, db=%#x\n", db)

	// ---- 3. exec DDL/DML with NULL callback --------------------------------
	var cErr uintptr // char **errmsg out-param
	sql := `CREATE TABLE langs (id INTEGER PRIMARY KEY, name TEXT, host TEXT);
	        INSERT INTO langs (name, host) VALUES ('clojure', 'jvm');
	        INSERT INTO langs (name, host) VALUES ('cljgo', 'go');`
	if rc := exec(db, sql, 0, 0, &cErr); rc != sqliteOK {
		msg := goString(cErr)
		freeFn(cErr)
		panic(fmt.Sprintf("sqlite3_exec rc=%d: %s", rc, msg))
	}
	fmt.Println("sqlite3_exec CREATE TABLE + 2x INSERT ok (callback=NULL)")

	// ---- 4. exec SELECT with a REAL Go callback (PATTERN 5+6) --------------
	// int cb(void *arg, int ncols, char **values, char **names)
	// Callback args must be pointer/integer kinds — no string auto-conversion
	// inside callbacks, so char** is walked manually with unsafe.
	rowsSeen := 0
	cb := purego.NewCallback(func(arg uintptr, ncols int32, values uintptr, names uintptr) int32 {
		rowsSeen++
		fmt.Printf("  exec-callback row %d:", rowsSeen)
		for i := 0; i < int(ncols); i++ {
			fmt.Printf(" %s=%s", goString(deref(names, i)), goString(deref(values, i)))
		}
		fmt.Println()
		return 0 // non-zero would abort the query (SQLITE_ABORT)
	})
	if rc := exec(db, "SELECT id, name, host FROM langs ORDER BY id", cb, 0, &cErr); rc != sqliteOK {
		panic(fmt.Sprintf("sqlite3_exec select rc=%d: %s", rc, errmsgFn(db)))
	}
	fmt.Printf("sqlite3_exec with Go callback ok (%d rows)\n", rowsSeen)

	// ---- 5. prepare/step/column (the non-callback row API) -----------------
	var stmt uintptr
	if rc := prepareV2(db, "SELECT name, host FROM langs WHERE host = 'go'", -1, &stmt, nil); rc != sqliteOK {
		panic(fmt.Sprintf("prepare_v2 rc=%d: %s", rc, errmsgFn(db)))
	}
	for {
		rc := step(stmt)
		if rc == sqliteRow {
			fmt.Printf("prepare/step row: name=%q host=%q\n", columnText(stmt, 0), columnText(stmt, 1))
			continue
		}
		if rc == sqliteDone {
			break
		}
		panic(fmt.Sprintf("step rc=%d: %s", rc, errmsgFn(db)))
	}
	finalize(stmt)

	// count via column_int for an int-typed read
	prepareV2(db, "SELECT count(*) FROM langs", -1, &stmt, nil)
	step(stmt)
	fmt.Printf("count(*) = %d\n", columnInt(stmt, 0))
	finalize(stmt)

	// ---- 6. close -----------------------------------------------------------
	if rc := closeDB(db); rc != sqliteOK {
		panic(fmt.Sprintf("close rc=%d", rc))
	}
	fmt.Println("sqlite3_close ok")

	// ---- 7. PATTERN 8: variadic C function — expected to MISBEHAVE ---------
	// char *sqlite3_mprintf(const char *fmt, ...) — Apple arm64 ABI passes
	// variadic args on the STACK; purego's RegisterFunc puts them in registers
	// as if they were fixed args. %d therefore reads garbage (not 42).
	var mprintf func(format string, args ...any) uintptr
	purego.RegisterLibFunc(&mprintf, lib, "sqlite3_mprintf")
	p := mprintf("variadic test: %d", 42)
	got := goString(p)
	freeFn(p)
	fmt.Printf("variadic sqlite3_mprintf(\"%%d\", 42) -> %q  (correct would be \"variadic test: 42\")\n", got)
	if got == "variadic test: 42" {
		fmt.Println("  -> variadic call WORKED (unexpected on darwin/arm64)")
	} else {
		fmt.Println("  -> variadic call returned garbage: confirmed purego limit on darwin/arm64")
	}
}
