package ipa

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"image"
	"image/png"
	"io"
)

func normalPNG(b []byte) ([]byte, error) {
	if !bytes.HasPrefix(b, pngSig) {
		return nil, errors.New("not a PNG")
	}
	var (
		cgbi          bool
		width, height int
		depth, color  byte
		interlace     byte
		idat          bytes.Buffer
	)
	for rest := b[len(pngSig):]; len(rest) >= 12; {
		n := int(binary.BigEndian.Uint32(rest))
		if n < 0 || len(rest) < 12+n {
			return nil, errors.New("truncated PNG")
		}
		typ, data := string(rest[4:8]), rest[8:8+n]
		rest = rest[12+n:]
		switch typ {
		case "CgBI":
			cgbi = true
		case "IHDR":
			if n < 13 {
				return nil, errors.New("short IHDR")
			}
			width, height = int(binary.BigEndian.Uint32(data)), int(binary.BigEndian.Uint32(data[4:]))
			depth, color, interlace = data[8], data[9], data[12]
		case "IDAT":
			idat.Write(data)
		case "IEND":
			rest = nil
		}
	}
	if !cgbi {
		if _, err := png.DecodeConfig(bytes.NewReader(b)); err != nil {
			return nil, err
		}
		return b, nil
	}

	bpp := 0
	switch {
	case depth == 8 && color == 6:
		bpp = 4
	case depth == 8 && color == 2:
		bpp = 3
	}
	if bpp == 0 || interlace != 0 || width <= 0 || height <= 0 || width > 4096 || height > 4096 {
		return nil, errors.New("a CgBI PNG of a kind this doesn't read")
	}
	stride := width * bpp
	raw := make([]byte, height*(1+stride))
	if _, err := io.ReadFull(flate.NewReader(&idat), raw); err != nil {
		return nil, err
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	prev := make([]byte, stride)
	for y := range height {
		line := raw[y*(1+stride) : (y+1)*(1+stride)]
		filter, cur := line[0], line[1:]
		if err := unfilter(filter, cur, prev, bpp); err != nil {
			return nil, err
		}
		for x := range width {
			p := cur[x*bpp:]
			o := img.Pix[y*img.Stride+x*4:]
			o[0], o[1], o[2], o[3] = p[2], p[1], p[0], 255
			if bpp == 4 {
				o[3] = p[3]
			}
		}
		prev = cur
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func unfilter(filter byte, cur, prev []byte, bpp int) error {
	switch filter {
	case 0:
	case 1:
		for i := bpp; i < len(cur); i++ {
			cur[i] += cur[i-bpp]
		}
	case 2:
		for i := range cur {
			cur[i] += prev[i]
		}
	case 3:
		for i := range cur {
			var left byte
			if i >= bpp {
				left = cur[i-bpp]
			}
			cur[i] += byte((int(left) + int(prev[i])) / 2)
		}
	case 4:
		for i := range cur {
			var a, c byte
			if i >= bpp {
				a, c = cur[i-bpp], prev[i-bpp]
			}
			cur[i] += paeth(a, prev[i], c)
		}
	default:
		return errors.New("unknown PNG filter")
	}
	return nil
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}
	return c
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
