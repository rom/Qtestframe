package auth

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// A small CBOR (RFC 8949) decoder for what WebAuthn needs: attestation
// objects, COSE keys and authenticator extensions. Authenticators encode
// them in the CTAP2 canonical form, so indefinite lengths are refused;
// nesting and sizes are bounded so that a hostile input cannot exhaust
// memory or the stack.
//
// Decoded values: unsigned and negative integers as int64 (values that do
// not fit are refused), byte strings as []byte, text strings as string,
// arrays as []any, maps as map[any]any with int64 or string keys, false,
// true and null as bool and nil, floats as float64. Tags are skipped (the
// tagged value is returned).

const (
	cborMaxDepth = 16
	cborMaxItems = 4096
)

var errCBOR = errors.New("cbor: malformed data")

func cborErr(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{errCBOR}, a...)...)
}

type cborDecoder struct {
	b     []byte
	pos   int
	items int
}

// cborDecode decodes the first CBOR data item of b and returns it with the
// number of bytes it took.
func cborDecode(b []byte) (any, int, error) {
	d := &cborDecoder{b: b}
	v, err := d.value(0)
	if err != nil {
		return nil, 0, err
	}
	return v, d.pos, nil
}

// cborDecodeAll decodes b, which must hold exactly one data item.
func cborDecodeAll(b []byte) (any, error) {
	v, n, err := cborDecode(b)
	if err != nil {
		return nil, err
	}
	if n != len(b) {
		return nil, cborErr("%d bytes after the data item", len(b)-n)
	}
	return v, nil
}

func (d *cborDecoder) byte() (byte, error) {
	if d.pos >= len(d.b) {
		return 0, cborErr("unexpected end of data")
	}
	c := d.b[d.pos]
	d.pos++
	return c, nil
}

func (d *cborDecoder) take(n uint64) ([]byte, error) {
	if n > uint64(len(d.b)-d.pos) {
		return nil, cborErr("length %d exceeds the data", n)
	}
	out := d.b[d.pos : d.pos+int(n)]
	d.pos += int(n)
	return out, nil
}

// head reads the initial byte and its argument.
func (d *cborDecoder) head() (major byte, info byte, arg uint64, err error) {
	c, err := d.byte()
	if err != nil {
		return 0, 0, 0, err
	}
	major, info = c>>5, c&0x1f
	switch {
	case info < 24:
		return major, info, uint64(info), nil
	case info == 24:
		x, err := d.take(1)
		if err != nil {
			return 0, 0, 0, err
		}
		return major, info, uint64(x[0]), nil
	case info == 25:
		x, err := d.take(2)
		if err != nil {
			return 0, 0, 0, err
		}
		return major, info, uint64(binary.BigEndian.Uint16(x)), nil
	case info == 26:
		x, err := d.take(4)
		if err != nil {
			return 0, 0, 0, err
		}
		return major, info, uint64(binary.BigEndian.Uint32(x)), nil
	case info == 27:
		x, err := d.take(8)
		if err != nil {
			return 0, 0, 0, err
		}
		return major, info, binary.BigEndian.Uint64(x), nil
	case info == 31:
		return 0, 0, 0, cborErr("indefinite lengths are not supported")
	}
	return 0, 0, 0, cborErr("reserved additional information %d", info)
}

func (d *cborDecoder) count() error {
	d.items++
	if d.items > cborMaxItems {
		return cborErr("more than %d data items", cborMaxItems)
	}
	return nil
}

func (d *cborDecoder) value(depth int) (any, error) {
	if depth > cborMaxDepth {
		return nil, cborErr("nested deeper than %d", cborMaxDepth)
	}
	if err := d.count(); err != nil {
		return nil, err
	}
	major, info, arg, err := d.head()
	if err != nil {
		return nil, err
	}
	switch major {
	case 0:
		if arg > math.MaxInt64 {
			return nil, cborErr("integer out of range")
		}
		return int64(arg), nil
	case 1:
		if arg > math.MaxInt64 {
			return nil, cborErr("integer out of range")
		}
		return -1 - int64(arg), nil
	case 2:
		b, err := d.take(arg)
		if err != nil {
			return nil, err
		}
		return append([]byte(nil), b...), nil
	case 3:
		b, err := d.take(arg)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	case 4:
		// every item takes at least one byte
		if arg > uint64(len(d.b)-d.pos) {
			return nil, cborErr("array length %d exceeds the data", arg)
		}
		out := make([]any, 0, int(arg))
		for i := uint64(0); i < arg; i++ {
			v, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case 5:
		if arg > uint64(len(d.b)-d.pos)/2 {
			return nil, cborErr("map length %d exceeds the data", arg)
		}
		out := make(map[any]any, int(arg))
		for i := uint64(0); i < arg; i++ {
			k, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			switch k.(type) {
			case int64, string:
			default:
				return nil, cborErr("map key of type %T", k)
			}
			if _, dup := out[k]; dup {
				return nil, cborErr("duplicate map key %v", k)
			}
			v, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			out[k] = v
		}
		return out, nil
	case 6:
		// a tag: its value
		return d.value(depth + 1)
	default: // 7: simple values and floats
		switch info {
		case 20:
			return false, nil
		case 21:
			return true, nil
		case 22, 23:
			return nil, nil
		case 25:
			return halfFloat(uint16(arg)), nil
		case 26:
			return float64(math.Float32frombits(uint32(arg))), nil
		case 27:
			return math.Float64frombits(arg), nil
		}
		return nil, cborErr("unsupported simple value %d", arg)
	}
}

// halfFloat converts an IEEE 754 half-precision value.
func halfFloat(h uint16) float64 {
	exp := int(h>>10) & 0x1f
	mant := float64(h & 0x3ff)
	var v float64
	switch exp {
	case 0:
		v = math.Ldexp(mant, -24)
	case 31:
		if mant == 0 {
			v = math.Inf(1)
		} else {
			v = math.NaN()
		}
	default:
		v = math.Ldexp(mant+1024, exp-25)
	}
	if h&0x8000 != 0 {
		return -v
	}
	return v
}
