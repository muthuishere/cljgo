package bri

// Internal tests: these reach newReadableStream directly, so they live in
// package bri rather than bri_test.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDrainedFileStreamReleasesItsHandle pins the fix for the windows-latest CI
// failure on b8b7718: a stream from of-file that is fully DRAINED must release
// the file, because a fully-read stream has nothing left to hold it for.
//
// It is expressed as a rename-and-remove rather than an fd count because that is
// exactly the operation Windows refuses while a handle is open ("The process
// cannot access the file because it is being used by another process"), and it
// is the operation the cljg-io-bytes conformance file performs. POSIX unlinks an
// open file happily, which is why macOS and Linux hid this for a full release
// cycle — so this test only FAILS on Windows, and that is the point: it is the
// regression guard for the platform that can actually detect it.
func TestDrainedFileStreamReleasesItsHandle(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "drain.bin")
	if err := os.WriteFile(p, []byte("hello bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	rs := newReadableStream(f, f)

	// Drain via the chunk path: read until EOF hands back nil.
	for {
		if chunk := rs.readBytes(4); chunk == nil {
			break
		}
	}

	// The handle must be gone WITHOUT an explicit Close().
	if !rs.closed {
		t.Fatal("a drained stream still holds its reader; Windows cannot delete the file")
	}
	if err := os.Remove(p); err != nil {
		t.Fatalf("removing a drained stream's file failed: %v", err)
	}

	// Idempotent: an explicit close after draining is still legal.
	if err := rs.Close(); err != nil {
		t.Fatalf("Close() after drain should be a no-op, got %v", err)
	}
}

// TestReadAllReleasesItsHandle is the same guarantee for the read-all path,
// which clojure.core/slurp also closes.
func TestReadAllReleasesItsHandle(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "slurp.txt")
	if err := os.WriteFile(p, []byte("line one\nline two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	rs := newReadableStream(f, f)

	if got := rs.readAll(); got != "line one\nline two\n" {
		t.Fatalf("read-all returned %q", got)
	}
	if !rs.closed {
		t.Fatal("read-all left the reader open")
	}
	if err := os.Remove(p); err != nil {
		t.Fatalf("removing a read-all'd file failed: %v", err)
	}
	// Reading past a drained stream stays quiet, not a panic.
	if got := rs.readAll(); got != "" {
		t.Fatalf("read-all after drain returned %q, want empty", got)
	}
}

// TestPartiallyReadStreamAtEOFReleasesHandle is the shape the drained-stream
// fix did NOT cover, and the one CI caught on windows-latest:
//
//	(first (st/chunks (st/of-file p)))
//
// The file is small enough to arrive in ONE chunk, so the read returns every
// byte with err == nil and EOF only shows up on a call that `first` never
// makes. The stream was therefore at end-of-file but never "drained", so the
// handle stayed open and the following delete failed with "The process cannot
// access the file because it is being used by another process".
//
// Expressed as remove-after-one-read rather than an fd count, because removal
// is exactly what Windows refuses while a handle is open. It passes on every
// platform but can only FAIL on Windows — deliberately, that being the
// platform able to detect it.
func TestPartiallyReadStreamAtEOFReleasesHandle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "one-chunk.bin")
	if err := os.WriteFile(path, []byte{0xff, 0x00, 0x41}, 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	rs := newReadableStream(f, f)

	// ONE read, consuming the whole file. No second read, no explicit close —
	// exactly what (first (chunks s)) does.
	if got := rs.readBytes(0); len(got) != 3 {
		t.Fatalf("readBytes = %v, want the 3 bytes written", got)
	}
	if !rs.closed {
		t.Fatal("stream is positioned at EOF after one read but still holds its handle")
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove after a single full read: %v — the handle leaked", err)
	}
}
