// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
// Package strictjson rejects ambiguous or lossy JSON before typed decoding.
// Callers retain responsibility for byte limits, schema and field validation.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Options struct {
	// MaxDepth defaults to 32. The root value has depth zero.
	MaxDepth int
	// FoldNames additionally rejects aliases matched case-insensitively by
	// encoding/json's struct decoder. Leave false for case-sensitive data maps.
	FoldNames bool
}

var (
	errSyntax    = errors.New("invalid or trailing JSON document")
	errUnicode   = errors.New("JSON contains invalid Unicode")
	errDuplicate = errors.New("JSON contains ambiguous duplicate fields")
	errDepth     = errors.New("JSON exceeds the nesting limit")
)

// Validate never includes source keys, values or snippets in its errors.
func Validate(data []byte, options Options) error {
	if !utf8.Valid(data) || !validEscapedUnicode(data) {
		return errUnicode
	}
	limit := options.MaxDepth
	if limit <= 0 {
		limit = 32
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var read func(int) error
	read = func(depth int) error {
		if depth > limit {
			return errDepth
		}
		token, err := decoder.Token()
		if err != nil {
			return errSyntax
		}
		delimiter, container := token.(json.Delim)
		if !container {
			return nil
		}
		var closing json.Delim
		switch delimiter {
		case '{':
			closing = '}'
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return errSyntax
				}
				name, ok := key.(string)
				if !ok {
					return errSyntax
				}
				if options.FoldNames {
					name = foldedName(name)
				}
				if seen[name] {
					return errDuplicate
				}
				seen[name] = true
				if err := read(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			closing = ']'
			for decoder.More() {
				if err := read(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errSyntax
		}
		end, err := decoder.Token()
		if err != nil || end != closing {
			return errSyntax
		}
		return nil
	}
	if err := read(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errSyntax
	}
	return nil
}

func foldedName(name string) string {
	return strings.Map(func(r rune) rune {
		minimum := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		return minimum
	}, name)
}

func hexWord(data []byte) (uint16, bool) {
	if len(data) < 4 {
		return 0, false
	}
	var word uint16
	for _, b := range data[:4] {
		word <<= 4
		switch {
		case b >= '0' && b <= '9':
			word |= uint16(b - '0')
		case b >= 'a' && b <= 'f':
			word |= uint16(b-'a') + 10
		case b >= 'A' && b <= 'F':
			word |= uint16(b-'A') + 10
		default:
			return 0, false
		}
	}
	return word, true
}

// encoding/json deliberately repairs unpaired UTF-16 escapes to U+FFFD. Check
// the original string lexemes first so medical/source text is never repaired.
// Ordinary escape syntax and document structure are checked by the decoder.
func validEscapedUnicode(data []byte) bool {
	for i := 0; i < len(data); {
		if data[i] != '"' {
			i++
			continue
		}
		i++
		for i < len(data) {
			if data[i] == '"' {
				i++
				break
			}
			if data[i] != '\\' {
				i++
				continue
			}
			i++
			if i >= len(data) {
				return false
			}
			if data[i] != 'u' {
				i++
				continue
			}
			word, ok := hexWord(data[i+1:])
			if !ok {
				return false
			}
			i += 5
			if word >= 0xdc00 && word <= 0xdfff {
				return false
			}
			if word >= 0xd800 && word <= 0xdbff {
				if i+6 > len(data) || data[i] != '\\' || data[i+1] != 'u' {
					return false
				}
				low, ok := hexWord(data[i+2:])
				if !ok || low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6
			}
		}
	}
	return true
}
