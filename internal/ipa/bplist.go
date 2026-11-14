package ipa

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
	"unicode/utf16"
)

func parseBinaryPlist(b []byte) (any, error) {
	if len(b) < 8+32 || string(b[:8]) != "bplist00" {
		return nil, errors.New("plist: not a bplist00")
	}
	t := b[len(b)-32:]
	offSize, refSize := int(t[6]), int(t[7])
	n := binary.BigEndian.Uint64(t[8:])
	top := binary.BigEndian.Uint64(t[16:])
	table := binary.BigEndian.Uint64(t[24:])
	end := uint64(len(b) - 32)
	if offSize < 1 || offSize > 8 || refSize < 1 || refSize > 8 ||
		table < 9 || table > end || n == 0 || n > (end-table)/uint64(offSize) || top >= n {
		return nil, errors.New("plist: bad trailer")
	}
	p := &bplist{
		objects: b[:table],
		offsets: b[table:end],
		offSize: offSize,
		refSize: refSize,
		n:       n,
		done:    map[uint64]any{},
		open:    map[uint64]bool{},
	}
	return p.object(top, 0)
}

type bplist struct {
	objects, offsets []byte
	offSize, refSize int
	n                uint64
	done             map[uint64]any
	open             map[uint64]bool
}

var errBplist = errors.New("plist: malformed bplist")

const apple = 978307200

func (p *bplist) object(i uint64, depth int) (any, error) {
	if i >= p.n {
		return nil, errBplist
	}
	if v, ok := p.done[i]; ok {
		return v, nil
	}
	if p.open[i] {
		return nil, errors.New("plist: an object contains itself")
	}
	if depth > 128 {
		return nil, errors.New("plist: nested too deep")
	}
	p.open[i] = true
	defer delete(p.open, i)
	off := uintN(p.offsets[int(i)*p.offSize:], p.offSize)
	if off < 8 || off >= uint64(len(p.objects)) {
		return nil, errBplist
	}
	v, err := p.value(int(off), depth)
	if err != nil {
		return nil, err
	}
	p.done[i] = v
	return v, nil
}

func (p *bplist) value(off, depth int) (any, error) {
	marker := p.objects[off]
	kind, low := marker>>4, int(marker&0xf)
	pos := off + 1
	switch kind {
	case 0x0:
		switch marker {
		case 0x08:
			return false, nil
		case 0x09:
			return true, nil
		}
		return nil, fmt.Errorf("plist: object 0x%02x", marker)
	case 0x1:
		v, _, err := p.integer(off)
		return v, err
	case 0x2:
		switch low {
		case 2:
			b, err := p.bytes(pos, 4)
			if err != nil {
				return nil, err
			}
			return float64(math.Float32frombits(binary.BigEndian.Uint32(b))), nil
		case 3:
			b, err := p.bytes(pos, 8)
			if err != nil {
				return nil, err
			}
			return math.Float64frombits(binary.BigEndian.Uint64(b)), nil
		}
		return nil, errBplist
	case 0x3:
		b, err := p.bytes(pos, 8)
		if err != nil || marker != 0x33 {
			return nil, errBplist
		}
		f := math.Float64frombits(binary.BigEndian.Uint64(b))
		if math.IsNaN(f) || math.Abs(f) > 1e12 {
			return nil, errors.New("plist: bad date")
		}
		sec := math.Floor(f)
		return time.Unix(apple+int64(sec), int64((f-sec)*1e9)).UTC(), nil
	case 0x8:
		b, err := p.bytes(pos, low+1)
		if err != nil {
			return nil, err
		}
		return map[string]any{"CF$UID": int64(uintN(b, low+1))}, nil
	}

	count, pos, err := p.count(low, pos)
	if err != nil {
		return nil, err
	}
	switch kind {
	case 0x4:
		b, err := p.bytes(pos, count)
		if err != nil {
			return nil, err
		}
		return append([]byte{}, b...), nil
	case 0x5:
		b, err := p.bytes(pos, count)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	case 0x6:
		if count > len(p.objects) {
			return nil, errBplist
		}
		b, err := p.bytes(pos, 2*count)
		if err != nil {
			return nil, err
		}
		u := make([]uint16, count)
		for i := range u {
			u[i] = binary.BigEndian.Uint16(b[2*i:])
		}
		return string(utf16.Decode(u)), nil
	case 0xA, 0xC:
		refs, err := p.refs(pos, count)
		if err != nil {
			return nil, err
		}
		a := make([]any, 0, count)
		for _, r := range refs {
			v, err := p.object(r, depth+1)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		return a, nil
	case 0xD:
		if count > len(p.objects) {
			return nil, errBplist
		}
		refs, err := p.refs(pos, 2*count)
		if err != nil {
			return nil, err
		}
		m := make(map[string]any, count)
		for i := range count {
			k, err := p.object(refs[i], depth+1)
			if err != nil {
				return nil, err
			}
			key, ok := k.(string)
			if !ok {
				return nil, errors.New("plist: a dict key that isn't a string")
			}
			v, err := p.object(refs[count+i], depth+1)
			if err != nil {
				return nil, err
			}
			m[key] = v
		}
		return m, nil
	}
	return nil, fmt.Errorf("plist: object 0x%02x", marker)
}

func (p *bplist) integer(off int) (int64, int, error) {
	marker := p.objects[off]
	if marker>>4 != 0x1 || marker&0xf > 4 {
		return 0, 0, errBplist
	}
	size := 1 << (marker & 0xf)
	b, err := p.bytes(off+1, size)
	if err != nil {
		return 0, 0, err
	}
	if size == 16 {
		high, low := uintN(b, 8), uintN(b[8:], 8)
		if (high != 0 || int64(low) < 0) && (high != math.MaxUint64 || int64(low) >= 0) {
			return 0, 0, errors.New("plist: integer too big")
		}
		return int64(low), off + 1 + size, nil
	}
	return int64(uintN(b, size)), off + 1 + size, nil
}

func (p *bplist) count(low, pos int) (int, int, error) {
	if low != 0xf {
		return low, pos, nil
	}
	if pos >= len(p.objects) {
		return 0, 0, errBplist
	}
	n, next, err := p.integer(pos)
	if err != nil {
		return 0, 0, err
	}
	if n < 0 || n > int64(len(p.objects)) {
		return 0, 0, errBplist
	}
	return int(n), next, nil
}

func (p *bplist) bytes(pos, n int) ([]byte, error) {
	if n < 0 || pos < 0 || pos > len(p.objects) || n > len(p.objects)-pos {
		return nil, errBplist
	}
	return p.objects[pos : pos+n], nil
}

func (p *bplist) refs(pos, n int) ([]uint64, error) {
	if n > len(p.objects) {
		return nil, errBplist
	}
	b, err := p.bytes(pos, n*p.refSize)
	if err != nil {
		return nil, err
	}
	refs := make([]uint64, n)
	for i := range refs {
		refs[i] = uintN(b[i*p.refSize:], p.refSize)
	}
	return refs, nil
}

func uintN(b []byte, n int) uint64 {
	var v uint64
	for _, c := range b[:n] {
		v = v<<8 | uint64(c)
	}
	return v
}
