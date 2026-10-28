// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// IHKE3206S01 passes Vue's iteration key, not a row's purported DATA01_SEQ,
// to the read-only SP_IHKE3206S08 operation. Keep this private selector outside
// the projected clinical fields, preserving number versus string semantics.
const providerDietKey = "\x00healthbank_diet_iteration_key"

func providerDietRows(raw []byte) ([]wireRow, error) {
	content := bytes.TrimSpace(raw)
	if len(content) == 0 {
		return nil, changedError()
	}
	if content[0] == '[' {
		rows, err := supplementRows(content, "")
		if err != nil {
			return nil, err
		}
		for index, row := range rows {
			if _, conflict := row[providerDietKey]; conflict {
				return nil, changedError()
			}
			row[providerDietKey] = json.RawMessage(strconv.Itoa(index))
		}
		return rows, nil
	}
	if content[0] != '{' {
		return nil, changedError()
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	if _, err := decoder.Token(); err != nil {
		return nil, changedError()
	}
	rows := []wireRow{}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, isString := token.(string)
		if err != nil || !isString || strings.TrimSpace(key) == "" || len(key) > 4096 || seen[key] || len(rows) >= MaxReportRecords {
			return nil, changedError()
		}
		seen[key] = true
		var source wireRow
		if decoder.Decode(&source) != nil || source == nil {
			return nil, changedError()
		}
		row, err := canonicalRow(source)
		if err != nil {
			return nil, err
		}
		if _, conflict := row[providerDietKey]; conflict {
			return nil, changedError()
		}
		row[providerDietKey], err = json.Marshal(key)
		if err != nil {
			return nil, changedError()
		}
		rows = append(rows, row)
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, changedError()
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, changedError()
	}
	return rows, nil
}

func providerDietDetailRequest(ref providerReference) providerRequest {
	var selector any = ref.rowID
	if ref.kind == "array_index" {
		selector = json.Number(ref.rowID)
	}
	header := providerSectionSpec("", "既有飲食記錄時間", "健康存摺／使用者自行記錄", "EATDATE_DATE", []fieldSpec{
		{"EATDATE_DATE", "來源記錄日期（原文）", ""}, {"EATDATE_HOUR", "來源記錄小時（原文）", ""}, {"EATDATE_MINUTE", "來源記錄分鐘（原文）", ""},
	})
	foods := providerSectionSpec("IsSelectFoodList", "既有記錄的食物選項", "健康存摺／使用者自行記錄（未推定實際攝取量）", "@parent", []fieldSpec{{"CODE", "來源已選食物代碼", ""}})
	return providerRequest{path: "/api/ihke3000/ihke3206s01/SP_IHKE3206S08", method: http.MethodPost,
		payload: map[string]any{"DATA01_SEQ": selector}, sections: []providerSection{header, foods}}
}
