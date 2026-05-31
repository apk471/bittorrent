package bencode

import (
	"bytes"
	"testing"
)

func TestDecodeInt(t *testing.T) {
	v, err := DecodeBytes([]byte("i42e"))
	if err != nil {
		t.Fatal(err)
	}
	i, ok := v.(Int)
	if !ok {
		t.Fatalf("expected Int, got %T", v)
	}
	if i != 42 {
		t.Fatalf("expected 42, got %d", i)
	}
}

func TestDecodeNegativeInt(t *testing.T) {
	v, err := DecodeBytes([]byte("i-42e"))
	if err != nil {
		t.Fatal(err)
	}
	i, ok := v.(Int)
	if !ok {
		t.Fatalf("expected Int, got %T", v)
	}
	if i != -42 {
		t.Fatalf("expected -42, got %d", i)
	}
}

func TestDecodeZero(t *testing.T) {
	v, err := DecodeBytes([]byte("i0e"))
	if err != nil {
		t.Fatal(err)
	}
	i, ok := v.(Int)
	if !ok {
		t.Fatalf("expected Int, got %T", v)
	}
	if i != 0 {
		t.Fatalf("expected 0, got %d", i)
	}
}

func TestDecodeString(t *testing.T) {
	v, err := DecodeBytes([]byte("4:spam"))
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(String)
	if !ok {
		t.Fatalf("expected String, got %T", v)
	}
	if string(s) != "spam" {
		t.Fatalf("expected 'spam', got %q", string(s))
	}
}

func TestDecodeEmptyString(t *testing.T) {
	v, err := DecodeBytes([]byte("0:"))
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(String)
	if !ok {
		t.Fatalf("expected String, got %T", v)
	}
	if len(s) != 0 {
		t.Fatalf("expected empty string, got %d bytes", len(s))
	}
}

func TestDecodeList(t *testing.T) {
	v, err := DecodeBytes([]byte("l4:spam4:eggse"))
	if err != nil {
		t.Fatal(err)
	}
	l, ok := v.(List)
	if !ok {
		t.Fatalf("expected List, got %T", v)
	}
	if len(l) != 2 {
		t.Fatalf("expected 2 items, got %d", len(l))
	}
	s1, ok := l[0].(String)
	if !ok {
		t.Fatalf("expected String at index 0, got %T", l[0])
	}
	if string(s1) != "spam" {
		t.Fatalf("expected 'spam', got %q", string(s1))
	}
	s2, ok := l[1].(String)
	if !ok {
		t.Fatalf("expected String at index 1, got %T", l[1])
	}
	if string(s2) != "eggs" {
		t.Fatalf("expected 'eggs', got %q", string(s2))
	}
}

func TestDecodeEmptyList(t *testing.T) {
	v, err := DecodeBytes([]byte("le"))
	if err != nil {
		t.Fatal(err)
	}
	l, ok := v.(List)
	if !ok {
		t.Fatalf("expected List, got %T", v)
	}
	if len(l) != 0 {
		t.Fatalf("expected empty list, got %d items", len(l))
	}
}

func TestDecodeListMixed(t *testing.T) {
	v, err := DecodeBytes([]byte("li1e2:ab3:cde4:eggse"))
	if err != nil {
		t.Fatal(err)
	}
	l, ok := v.(List)
	if !ok {
		t.Fatalf("expected List, got %T", v)
	}
	if len(l) != 4 {
		t.Fatalf("expected 4 items, got %d", len(l))
	}
	if l[0] != Int(1) {
		t.Fatal("expected first element 1")
	}
	if string(l[1].(String)) != "ab" {
		t.Fatal("expected second element 'ab'")
	}
	if string(l[2].(String)) != "cde" {
		t.Fatal("expected third element 'cde'")
	}
	if string(l[3].(String)) != "eggs" {
		t.Fatal("expected fourth element 'eggs'")
	}
}

func TestDecodeDict(t *testing.T) {
	v, err := DecodeBytes([]byte("d3:cow3:moo4:spam4:eggse"))
	if err != nil {
		t.Fatal(err)
	}
	d, ok := v.(Dict)
	if !ok {
		t.Fatalf("expected Dict, got %T", v)
	}
	if len(d) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(d))
	}
	cow, ok := d["cow"].(String)
	if !ok || string(cow) != "moo" {
		t.Fatalf("expected cow=moo, got cow=%q", string(cow))
	}
	spam, ok := d["spam"].(String)
	if !ok || string(spam) != "eggs" {
		t.Fatalf("expected spam=eggs, got spam=%q", string(spam))
	}
}

func TestDecodeEmptyDict(t *testing.T) {
	v, err := DecodeBytes([]byte("de"))
	if err != nil {
		t.Fatal(err)
	}
	d, ok := v.(Dict)
	if !ok {
		t.Fatalf("expected Dict, got %T", v)
	}
	if len(d) != 0 {
		t.Fatalf("expected empty dict, got %d keys", len(d))
	}
}

func TestDecodeNestedStructures(t *testing.T) {
	data := []byte("d4:listli1ei2ei3ee3:numi42ee")
	v, err := DecodeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := v.(Dict)
	if !ok {
		t.Fatalf("expected Dict, got %T", v)
	}
	list, _ := d.GetList("list")
	if len(list) != 3 {
		t.Fatalf("expected list with 3 items, got %d", len(list))
	}
	num, _ := d.GetInt("num")
	if num != 42 {
		t.Fatalf("expected num=42, got %d", num)
	}
}

func TestDecodeDictWithListThenKey(t *testing.T) {
	input := []byte("d5:filesl8:file.txt9:image.jpge4:sizei2048ee")
	v, err := DecodeBytes(input)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := v.(Dict)
	if !ok {
		t.Fatalf("expected Dict, got %T", v)
	}
	if len(d) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(d))
	}
	if _, ok := d["size"]; !ok {
		t.Fatal("key 'size' is missing!")
	}
	if _, ok := d["files"]; !ok {
		t.Fatal("key 'files' is missing!")
	}
}

func TestDecodeDictWithIntThenList(t *testing.T) {
	input := []byte("d4:name10:ubuntu.iso6:lengthi123456e5:filesl9:part1.iso9:part2.isoee")
	v, err := DecodeBytes(input)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := v.(Dict)
	if !ok {
		t.Fatalf("expected Dict, got %T", v)
	}
	if len(d) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(d))
	}
}

func TestRoundTripInt(t *testing.T) {
	orig := Int(12345)
	var buf bytes.Buffer
	if err := Encode(&buf, orig); err != nil {
		t.Fatal(err)
	}
	v, err := Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if v != orig {
		t.Fatalf("expected %d, got %d", orig, v)
	}
}

func TestRoundTripString(t *testing.T) {
	orig := String("hello world")
	var buf bytes.Buffer
	if err := Encode(&buf, orig); err != nil {
		t.Fatal(err)
	}
	v, err := Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(v.(String), orig) {
		t.Fatalf("expected %q, got %q", orig, v.(String))
	}
}

func TestRoundTripList(t *testing.T) {
	orig := List{Int(1), String("two"), Int(3)}
	var buf bytes.Buffer
	if err := Encode(&buf, orig); err != nil {
		t.Fatal(err)
	}
	v, err := Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok := v.(List)
	if !ok || len(decoded) != 3 {
		t.Fatalf("unexpected result: %v", v)
	}
	if decoded[0] != Int(1) {
		t.Fatal("expected first element 1")
	}
	if !bytes.Equal(decoded[1].(String), String("two")) {
		t.Fatal("expected second element 'two'")
	}
	if decoded[2] != Int(3) {
		t.Fatal("expected third element 3")
	}
}

func TestRoundTripDict(t *testing.T) {
	orig := Dict{
		"b": Int(2),
		"a": Int(1),
		"c": Int(3),
	}
	var buf bytes.Buffer
	if err := Encode(&buf, orig); err != nil {
		t.Fatal(err)
	}
	v, err := Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok := v.(Dict)
	if !ok || len(decoded) != 3 {
		t.Fatalf("unexpected result: %v", v)
	}
}

func TestDecodeBinaryString(t *testing.T) {
	data := make([]byte, 0)
	data = append(data, []byte("20:")...)
	raw := make([]byte, 20)
	for i := range raw {
		raw[i] = byte(i)
	}
	data = append(data, raw...)
	v, err := DecodeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := v.(String)
	if !ok {
		t.Fatalf("expected String, got %T", v)
	}
	if len(s) != 20 {
		t.Fatalf("expected 20 bytes, got %d", len(s))
	}
	for i, b := range s {
		if b != byte(i) {
			t.Fatalf("byte %d: expected %d, got %d", i, i, b)
		}
	}
}

func TestRejectInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"just i", []byte("i")},
		{"just l", []byte("l")},
		{"just d", []byte("d")},
		{"unclosed int", []byte("i42")},
		{"unclosed list", []byte("li1e")},
		{"unclosed dict", []byte("d3:cow3:moo")},
		{"negative length string", []byte("-1:")},
		{"garbage", []byte("xyz")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := DecodeBytes(c.data)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
