package bencode

import "fmt"

// Value is a bencode value. The concrete types are Int, String, List, and Dict.
type Value interface {
	value()
}

// Int is a bencode integer.
type Int int64

// String is a bencode byte string.
type String []byte

// List is a bencode list.
type List []Value

// Dict is a bencode dictionary (keys are strings, values are bencode values).
type Dict map[string]Value

func (Int) value()    {}
func (String) value() {}
func (List) value()   {}
func (Dict) value()   {}

// String returns the byte string as a Go string.
func (s String) String() string {
	return string(s)
}

// Bytes returns the byte string as a Go []byte.
func (s String) Bytes() []byte {
	return []byte(s)
}

// GetString looks up a key in the dict and returns it as a String.
func (d Dict) GetString(key string) (String, error) {
	v, ok := d[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found", key)
	}
	s, ok := v.(String)
	if !ok {
		return nil, fmt.Errorf("value for key %q is not a string", key)
	}
	return s, nil
}

// GetInt looks up a key in the dict and returns it as an Int.
func (d Dict) GetInt(key string) (Int, error) {
	v, ok := d[key]
	if !ok {
		return 0, fmt.Errorf("key %q not found", key)
	}
	i, ok := v.(Int)
	if !ok {
		return 0, fmt.Errorf("value for key %q is not an int", key)
	}
	return i, nil
}

// GetList looks up a key in the dict and returns it as a List.
func (d Dict) GetList(key string) (List, error) {
	v, ok := d[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found", key)
	}
	l, ok := v.(List)
	if !ok {
		return nil, fmt.Errorf("value for key %q is not a list", key)
	}
	return l, nil
}

// GetDict looks up a key in the dict and returns it as a Dict.
func (d Dict) GetDict(key string) (Dict, error) {
	v, ok := d[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found", key)
	}
	dd, ok := v.(Dict)
	if !ok {
		return nil, fmt.Errorf("value for key %q is not a dict", key)
	}
	return dd, nil
}
