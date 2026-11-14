package ipa

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

func parsePlist(b []byte) (any, error) {
	if bytes.HasPrefix(b, []byte("bplist")) {
		return parseBinaryPlist(b)
	}
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = false
	for {
		t, err := d.Token()
		if err != nil {
			return nil, fmt.Errorf("plist: %w", err)
		}
		if se, ok := t.(xml.StartElement); ok {
			if se.Name.Local != "plist" {
				return nil, errors.New("plist: no <plist>")
			}
			for {
				t, err := d.Token()
				if err != nil {
					return nil, fmt.Errorf("plist: %w", err)
				}
				if se, ok := t.(xml.StartElement); ok {
					return plistValue(d, se)
				}
			}
		}
	}
}

func plistValue(d *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "dict":
		m := map[string]any{}
		key := ""
		for {
			t, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := t.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					s, err := plistText(d)
					if err != nil {
						return nil, err
					}
					key = s
					continue
				}
				v, err := plistValue(d, t)
				if err != nil {
					return nil, err
				}
				m[key] = v
			case xml.EndElement:
				return m, nil
			}
		}
	case "array":
		a := []any{}
		for {
			t, err := d.Token()
			if err != nil {
				return nil, err
			}
			switch t := t.(type) {
			case xml.StartElement:
				v, err := plistValue(d, t)
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			case xml.EndElement:
				return a, nil
			}
		}
	case "true", "false":
		if err := d.Skip(); err != nil {
			return nil, err
		}
		return start.Name.Local == "true", nil
	}
	s, err := plistText(d)
	if err != nil {
		return nil, err
	}
	switch start.Name.Local {
	case "string":
		return s, nil
	case "integer":
		return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	case "real":
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	case "date":
		return time.Parse(time.RFC3339, strings.TrimSpace(s))
	case "data":
		return base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
	}
	return nil, fmt.Errorf("plist: unknown <%s>", start.Name.Local)
}

func plistText(d *xml.Decoder) (string, error) {
	var b strings.Builder
	for {
		t, err := d.Token()
		if err == io.EOF {
			return "", io.ErrUnexpectedEOF
		}
		if err != nil {
			return "", err
		}
		switch t := t.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			return b.String(), nil
		case xml.StartElement:
			return "", fmt.Errorf("plist: <%s> inside text", t.Name.Local)
		}
	}
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func dict(m map[string]any, k string) map[string]any {
	d, _ := m[k].(map[string]any)
	return d
}

func strs(m map[string]any, k string) []string {
	a, _ := m[k].([]any)
	var out []string
	for _, v := range a {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func boolean(m map[string]any, k string) bool {
	b, _ := m[k].(bool)
	return b
}
