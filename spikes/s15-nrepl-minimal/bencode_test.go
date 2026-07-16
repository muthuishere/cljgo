package main

import (
	"bufio"
	"bytes"
	"reflect"
	"testing"
)

func TestBencodeRoundTrip(t *testing.T) {
	msg := map[string]any{
		"op":      "eval",
		"code":    "(+ 1 2)",
		"id":      "1",
		"line":    int64(42),
		"status":  []any{"done"},
		"nested":  map[string]any{"a": "b"},
		"unicode": "λ→\"quoted\"",
	}
	var buf bytes.Buffer
	if err := bencodeWrite(&buf, msg); err != nil {
		t.Fatal(err)
	}
	got, err := bencodeRead(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, msg) {
		t.Fatalf("round trip mismatch:\n got %#v\nwant %#v", got, msg)
	}
}

func TestBencodeDictKeysSorted(t *testing.T) {
	var buf bytes.Buffer
	if err := bencodeWrite(&buf, map[string]any{"b": "2", "a": "1", "c": "3"}); err != nil {
		t.Fatal(err)
	}
	want := "d1:a1:11:b1:21:c1:3e"
	if buf.String() != want {
		t.Fatalf("got %q want %q", buf.String(), want)
	}
}
