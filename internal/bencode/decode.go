package bencode

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
)

type decoder struct {
	r *bufio.Reader
}

func Decode(r io.Reader) (Value, error) {
	d := &decoder{r: bufio.NewReader(r)}
	v, err := d.decodeValue()
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (d *decoder) decodeValue() (Value, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return nil, err
	}
	if err := d.r.UnreadByte(); err != nil {
		return nil, err
	}

	switch {
	case b >= '0' && b <= '9':
		return d.decodeString()
	case b == 'i':
		return d.decodeInt()
	case b == 'l':
		return d.decodeList()
	case b == 'd':
		return d.decodeDict()
	default:
		return nil, fmt.Errorf("bencode: unexpected byte %q", b)
	}
}

func (d *decoder) decodeString() (String, error) {
	lenStr, err := d.readUntil(':')
	if err != nil {
		return nil, err
	}
	length, err := strconv.Atoi(string(lenStr))
	if err != nil {
		return nil, fmt.Errorf("bencode: invalid string length %q", lenStr)
	}
	if length < 0 {
		return nil, fmt.Errorf("bencode: negative string length %d", length)
	}
	buf := make([]byte, length)
	_, err = io.ReadFull(d.r, buf)
	if err != nil {
		return nil, fmt.Errorf("bencode: reading string content: %w", err)
	}
	return String(buf), nil
}

func (d *decoder) decodeInt() (Int, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return 0, err
	}
	if b != 'i' {
		return 0, fmt.Errorf("bencode: expected 'i' at start of int")
	}
	numStr, err := d.readUntil('e')
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(string(numStr), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bencode: invalid integer %q", numStr)
	}
	return Int(n), nil
}

func (d *decoder) decodeList() (List, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return nil, err
	}
	if b != 'l' {
		return nil, fmt.Errorf("bencode: expected 'l' at start of list")
	}
	var list List
	for {
		peek, err := d.r.ReadByte()
		if err != nil {
			return nil, err
		}
		if err := d.r.UnreadByte(); err != nil {
			return nil, err
		}
		if peek == 'e' {
			d.r.ReadByte()
			break
		}
		v, err := d.decodeValue()
		if err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, nil
}

func (d *decoder) decodeDict() (Dict, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return nil, err
	}
	if b != 'd' {
		return nil, fmt.Errorf("bencode: expected 'd' at start of dict")
	}
	dict := make(Dict)
	for {
		peek, err := d.r.ReadByte()
		if err != nil {
			return nil, err
		}
		if err := d.r.UnreadByte(); err != nil {
			return nil, err
		}
		if peek == 'e' {
			d.r.ReadByte()
			break
		}
		key, err := d.decodeString()
		if err != nil {
			return nil, err
		}
		val, err := d.decodeValue()
		if err != nil {
			return nil, err
		}
		dict[string(key)] = val
	}
	return dict, nil
}

func (d *decoder) readUntil(stop byte) ([]byte, error) {
	var buf []byte
	for {
		b, err := d.r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == stop {
			return buf, nil
		}
		buf = append(buf, b)
	}
}

func DecodeBytes(data []byte) (Value, error) {
	return Decode(bytes.NewReader(data))
}
