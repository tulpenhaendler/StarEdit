package save

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// The save is one ~17 MB JSON document. Round-tripping it through
// encoding/json would reorder keys and reformat every float, so instead we
// split only the objects on the path to what we edit and keep everything else
// as the original bytes.

// object is a JSON object split into raw key/value pairs, order preserved.
type object struct {
	keys []string
	vals [][]byte
}

// skipValue returns the index just past the JSON value starting at b[i].
func skipValue(b []byte, i int) (int, error) {
	i = skipSpace(b, i)
	if i >= len(b) {
		return 0, fmt.Errorf("unexpected end of JSON")
	}
	switch b[i] {
	case '"':
		return skipString(b, i)
	case '{', '[':
		depth := 0
		for i < len(b) {
			switch b[i] {
			case '"':
				j, err := skipString(b, i)
				if err != nil {
					return 0, err
				}
				i = j
				continue
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return i + 1, nil
				}
			}
			i++
		}
		return 0, fmt.Errorf("unterminated JSON container")
	default:
		for i < len(b) && !strings.ContainsRune(",}] \t\r\n", rune(b[i])) {
			i++
		}
		return i, nil
	}
}

func skipString(b []byte, i int) (int, error) {
	for i++; i < len(b); i++ {
		switch b[i] {
		case '\\':
			i++
		case '"':
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated JSON string")
}

func skipSpace(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\r' || b[i] == '\n') {
		i++
	}
	return i
}

func parseObject(b []byte) (*object, error) {
	i := skipSpace(b, 0)
	if i >= len(b) || b[i] != '{' {
		return nil, fmt.Errorf("expected JSON object")
	}
	o := &object{}
	i = skipSpace(b, i+1)
	if i < len(b) && b[i] == '}' {
		return o, nil
	}
	for {
		i = skipSpace(b, i)
		end, err := skipValue(b, i)
		if err != nil {
			return nil, err
		}
		var key string
		if err := json.Unmarshal(b[i:end], &key); err != nil {
			return nil, fmt.Errorf("bad object key: %w", err)
		}
		i = skipSpace(b, end)
		if i >= len(b) || b[i] != ':' {
			return nil, fmt.Errorf("expected ':' after key %q", key)
		}
		i = skipSpace(b, i+1)
		end, err = skipValue(b, i)
		if err != nil {
			return nil, err
		}
		o.keys = append(o.keys, key)
		o.vals = append(o.vals, b[i:end])
		i = skipSpace(b, end)
		if i >= len(b) {
			return nil, fmt.Errorf("unterminated JSON object")
		}
		if b[i] == '}' {
			return o, nil
		}
		if b[i] != ',' {
			return nil, fmt.Errorf("expected ',' in object, got %q", b[i])
		}
		i++
	}
}

func (o *object) get(key string) ([]byte, bool) {
	for i, k := range o.keys {
		if k == key {
			return o.vals[i], true
		}
	}
	return nil, false
}

func (o *object) set(key string, val []byte) {
	for i, k := range o.keys {
		if k == key {
			o.vals[i] = val
			return
		}
	}
	o.keys = append(o.keys, key)
	o.vals = append(o.vals, val)
}

func (o *object) bytes() []byte {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(marshal(k))
		buf.WriteByte(':')
		buf.Write(o.vals[i])
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

// child parses the object stored under key.
func (o *object) child(key string) (*object, error) {
	raw, ok := o.get(key)
	if !ok {
		return nil, fmt.Errorf("key %q not found", key)
	}
	c, err := parseObject(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return c, nil
}

// marshal encodes v compactly without HTML escaping, matching the game's output.
func marshal(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}
