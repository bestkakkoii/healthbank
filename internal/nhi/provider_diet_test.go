// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProviderDietIterationKeysAndReadOnlyMealDetail(t *testing.T) {
	for _, tc := range []struct {
		name, selectorJSON string
		list               any
	}{
		{"array_index", "0", []any{map[string]any{"DATA01_SEQ": "WRONG_ROW_ID", "eatDate": "115/10/01 08:30", "foodName": "合成食物名稱", "totalCal": "123.000000000000001"}}},
		{"object_key", `"SYNTHETIC-MEAL-KEY"`, map[string]any{"SYNTHETIC-MEAL-KEY": map[string]any{"DATA01_SEQ": "WRONG_ROW_ID", "eatDate": "115/10/01 08:30", "foodName": "合成食物名稱", "totalCal": "123.000000000000001"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var detailCalls atomic.Int32
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.RawQuery != "" {
					t.Error("diet read method or range changed")
				}
				switch r.URL.Path {
				case "/api/ihke3000/ihke3206s01/IHKE3206S01":
					respondJSON(t, w, map[string]any{"dietRecordList": tc.list, "foodList": []any{map[string]any{"CODE": "SYNTHETIC-FOOD", "CODE_NAME": "未取得實際攝取量的來源目錄", "CAL": "9999999"}}})
				case "/api/ihke3000/ihke3206s01/SP_IHKE3206S08":
					detailCalls.Add(1)
					var payload map[string]json.RawMessage
					if json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload) != 1 || string(payload["DATA01_SEQ"]) != tc.selectorJSON {
						t.Error("diet selector was invented, taken from row fields, or changed JSON type")
					}
					respondJSON(t, w, map[string]any{"ResultStatus": true, "EATDATE_DATE": "2026-10-01", "EATDATE_HOUR": "08", "EATDATE_MINUTE": "30", "IsSelectFoodList": []any{map[string]any{"CODE": "SYNTHETIC-FOOD", "jwt_token": "NEVER-EXPOSE"}, map[string]any{"CODE": "SYNTHETIC-FOOD"}}})
				default:
					t.Error("unexpected or mutating diet route")
				}
			})
			list, err := s.FetchProvider(context.Background(), "diet", Query{StartDate: "1912-01-01", EndDate: "2026-10-28"})
			if err != nil || len(list.Entries) != 1 || list.Entries[0].DetailID == "" || list.StartDate != "" {
				t.Fatalf("diet source/keyed records failed: %v", err)
			}
			ref := list.Entries[0].DetailID
			detail, err := s.FetchProviderDetail(context.Background(), "diet", ref)
			if err != nil || len(detail.Entries) != 3 || detail.Complete || detail.StartDate != "" || detail.Entries[0].Date != "2026-10-01" || detail.Entries[1].Date != "115/10/01 08:30" || detail.Entries[1].Key == detail.Entries[2].Key {
				t.Fatalf("diet date components, duplicate selections or coverage changed: %v", err)
			}
			body, _ := json.Marshal([]SupplementReport{list, detail})
			for _, forbidden := range []string{"NEVER-EXPOSE", "WRONG_ROW_ID", "SYNTHETIC-MEAL-KEY", "9999999", "healthbank_diet_iteration_key"} {
				if strings.Contains(string(body), forbidden) {
					t.Fatal("diet private selector, credential or unobserved intake escaped projection")
				}
			}
			if !strings.Contains(string(body), "123.000000000000001") || !strings.Contains(string(body), `"value":"08"`) || !strings.Contains(string(body), `"value":"30"`) {
				t.Fatal("source calories or time precision was rewritten")
			}
			if _, err := s.FetchProviderDetail(context.Background(), "exercise", ref); err == nil || detailCalls.Load() != 1 {
				t.Fatal("meal reference escaped category binding")
			}
			if _, err := s.FetchProvider(context.Background(), "diet", Query{}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.FetchProviderDetail(context.Background(), "diet", ref); err == nil || detailCalls.Load() != 1 {
				t.Fatal("old meal selector survived a successful list refresh")
			}
		})
	}
}

func TestProviderDietRejectsMalformedListsAndFailedDetails(t *testing.T) {
	for _, body := range []string{
		`{"dietRecordList":{"same":{"eatDate":"115/01/01"},"same":{"eatDate":"115/01/02"}}}`,
		`{"dietRecordList":{"a":null}}`,
		`{"dietRecordList":{"a":1}}`,
		`{"dietRecordList":{"":{"eatDate":"115/01/01"}}}`,
		`{"dietRecordList":[{"eatDate":"115/01/01","\u0000healthbank_diet_iteration_key":"FORGED"}]}`,
		`{"dietRecordList":true}`,
	} {
		t.Run(body, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
			if _, err := s.FetchProvider(context.Background(), "diet", Query{}); err == nil || len(s.providerReferences["diet"]) != 0 {
				t.Fatal("ambiguous or malformed diet list created live references")
			}
		})
	}
	for _, result := range []any{
		map[string]any{"ResultStatus": false, "ResultMsg": "PRIVATE-ERROR-MUST-NOT-LEAK"},
		map[string]any{"ResultStatus": "true", "EATDATE_DATE": "2026-10-01", "IsSelectFoodList": []any{}},
		map[string]any{"ResultStatus": true, "EATDATE_DATE": "2026-10-01"},
		map[string]any{"ResultStatus": true, "EATDATE_DATE": "2026-10-01", "IsSelectFoodList": "changed"},
	} {
		s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/ihke3000/ihke3206s01/IHKE3206S01" {
				respondJSON(t, w, map[string]any{"dietRecordList": []any{map[string]any{"eatDate": "115/10/01", "foodName": "合成"}}})
			} else {
				respondJSON(t, w, result)
			}
		})
		list, err := s.FetchProvider(context.Background(), "diet", Query{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.FetchProviderDetail(context.Background(), "diet", list.Entries[0].DetailID); err == nil || strings.Contains(err.Error(), "PRIVATE-ERROR") {
			t.Fatal("failed or incomplete diet detail became success or exposed raw error")
		}
	}
}
