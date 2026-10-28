// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package strictjson

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRejectAmbiguousAndLossyDocuments(t *testing.T) {
	for _, input := range []string{
		`{"result":"1","result":"2"}`, `{"result":"1","re\u0073ult":"2"}`,
		`{"nested":[{"value":1,"value":2}]}`, `{"value":"\ud800"}`,
		`{"value":"\udc00"}`, `{"value":"\ud800\u0041"}`,
		`{"value":"\ud800\\udc00"}`, `{"\ud800":"key"}`,
		"{\"value\":\"\xff\"}", `{} []`, ``, `{"x":}`, `[1,]`, `{"x":"\u12"}`,
		strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34),
	} {
		if err := Validate([]byte(input), Options{}); err == nil {
			t.Fatalf("accepted invalid synthetic document %q", input)
		}
	}
}

func TestPreserveValidUnicodeAndMapKeys(t *testing.T) {
	for _, input := range []string{
		`{"text":"中文與𠮷，原文�"}`, `{"text":"\ud842\udfb7"}`,
		`{"text":"literal \\ud800 and escaped \"quote\""}`,
		`{"text":"\u005cud800"}`, `{"key":"a","KEY":"b"}`,
		`{"x":[1e99,null,true,false,{},[]]}`, `"\u0000\t\r\n"`, `42`,
	} {
		if err := Validate([]byte(input), Options{}); err != nil {
			t.Fatalf("rejected valid synthetic document %q: %v", input, err)
		}
	}
	for _, input := range []string{`{"key":1,"KEY":2}`, `{"k":1,"K":2}`, `{"s":1,"ſ":2}`} {
		if err := Validate([]byte(input), Options{FoldNames: true}); err == nil {
			t.Fatal("accepted an encoding/json case-fold alias")
		}
	}
	if err := Validate([]byte(`{"a":{"b":1}}`), Options{MaxDepth: 1}); err == nil {
		t.Fatal("ignored caller nesting limit")
	}
}

func FuzzValidationNeverAcceptsInvalidJSON(f *testing.F) {
	for _, seed := range []string{`{"result":"合成資料"}`, `{"x":"\ud842\udfb7"}`, `{"x":"\ud800"}`, `[1,2]`, `"\\\""`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 1<<16 {
			t.Skip()
		}
		if Validate([]byte(input), Options{}) == nil && !json.Valid([]byte(input)) {
			t.Fatal("validator accepted invalid JSON")
		}
	})
}
