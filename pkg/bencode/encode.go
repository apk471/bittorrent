package bencode

import (
	"bytes"
	"fmt"
	"io"
	"sort"
)

// Encode writes the bencode-encoded form of v to w.
func Encode(w io.Writer, v Value) error {
	switch val := v.(type) {
	case Int:
		_, err := fmt.Fprintf(w, "i%de", int64(val))
		return err
	case String:
		_, err := fmt.Fprintf(w, "%d:", len(val))
		if err != nil {
			return err
		}
		_, err = w.Write(val)
		return err
	case List:
		if _, err := io.WriteString(w, "l"); err != nil {
			return err
		}
		for _, item := range val {
			if err := Encode(w, item); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, "e")
		return err
	case Dict:
		if _, err := io.WriteString(w, "d"); err != nil {
			return err
		}
		// keys must be sorted lexicographically per BEP-3
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := Encode(w, String(k)); err != nil {
				return err
			}
			if err := Encode(w, val[k]); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, "e")
		return err
	default:
		return fmt.Errorf("bencode: unknown type %T", v)
	}
}

// EncodeBytes encodes v to bencode and returns the byte slice.
func EncodeBytes(v Value) ([]byte, error) {
	var buf bytes.Buffer
	if err := Encode(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
