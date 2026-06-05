package bencode

import "fmt"

type Value interface {
	value()
}

type Int int64
type String []byte
type List []Value
type Dict map[string]Value

func (Int) value()    {}
func (String) value() {}
func (List) value()   {}
func (Dict) value()   {}

func (s String) String() string {
	return string(s)
}

func (s String) Bytes() []byte {
	return []byte(s)
}

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
