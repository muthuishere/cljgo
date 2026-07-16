// Bencode codec for nREPL's wire format — written from scratch to
// answer the S15 dependency question: is a codec small enough to own?
// (Answer: this file. Encode+decode of everything nREPL uses — byte
// strings, integers, lists, dicts with sorted keys — in ~150 lines.)
//
// Mapping: Go string <-> bencode byte string, int64 <-> integer,
// []any <-> list, map[string]any <-> dict. nREPL never nests deeper
// than dict->list->dict and never uses non-string dict keys.
package main

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
)

// ---------------------------------------------------------------- encode

func bencodeWrite(w io.Writer, v any) error {
	switch x := v.(type) {
	case string:
		_, err := fmt.Fprintf(w, "%d:%s", len(x), x)
		return err
	case []byte:
		if _, err := fmt.Fprintf(w, "%d:", len(x)); err != nil {
			return err
		}
		_, err := w.Write(x)
		return err
	case int:
		_, err := fmt.Fprintf(w, "i%de", x)
		return err
	case int64:
		_, err := fmt.Fprintf(w, "i%de", x)
		return err
	case []any:
		if _, err := io.WriteString(w, "l"); err != nil {
			return err
		}
		for _, e := range x {
			if err := bencodeWrite(w, e); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, "e")
		return err
	case []string: // convenience: status lists, session lists
		l := make([]any, len(x))
		for i, s := range x {
			l[i] = s
		}
		return bencodeWrite(w, l)
	case map[string]any:
		if _, err := io.WriteString(w, "d"); err != nil {
			return err
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys) // bencode dicts are key-sorted
		for _, k := range keys {
			if err := bencodeWrite(w, k); err != nil {
				return err
			}
			if err := bencodeWrite(w, x[k]); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, "e")
		return err
	default:
		return fmt.Errorf("bencode: cannot encode %T", v)
	}
}

// ---------------------------------------------------------------- decode

func bencodeRead(r *bufio.Reader) (any, error) {
	c, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch {
	case c == 'i':
		digits, err := r.ReadString('e')
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(digits[:len(digits)-1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bencode: bad integer %q", digits)
		}
		return n, nil
	case c == 'l':
		var list []any
		for {
			p, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			if p == 'e' {
				return list, nil
			}
			if err := r.UnreadByte(); err != nil {
				return nil, err
			}
			e, err := bencodeRead(r)
			if err != nil {
				return nil, err
			}
			list = append(list, e)
		}
	case c == 'd':
		dict := map[string]any{}
		for {
			p, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			if p == 'e' {
				return dict, nil
			}
			if err := r.UnreadByte(); err != nil {
				return nil, err
			}
			k, err := bencodeRead(r)
			if err != nil {
				return nil, err
			}
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("bencode: non-string dict key %T", k)
			}
			v, err := bencodeRead(r)
			if err != nil {
				return nil, err
			}
			dict[ks] = v
		}
	case c >= '0' && c <= '9':
		if err := r.UnreadByte(); err != nil {
			return nil, err
		}
		lenStr, err := r.ReadString(':')
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(lenStr[:len(lenStr)-1])
		if err != nil || n < 0 {
			return nil, fmt.Errorf("bencode: bad string length %q", lenStr)
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf), nil
	default:
		return nil, fmt.Errorf("bencode: unexpected byte %q", c)
	}
}
