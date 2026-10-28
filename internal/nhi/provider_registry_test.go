// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/strictjson"
)

type reviewedProviderSection struct {
	Root     string   `json:"root"`
	Shape    string   `json:"shape"`
	Date     string   `json:"date_field"`
	End      string   `json:"end_date_field"`
	Fields   []string `json:"fields"`
	Units    []string `json:"units"`
	Optional bool     `json:"optional"`
	Metadata bool     `json:"metadata,omitempty"`
}

type reviewedProviderRequest struct {
	Path          string                    `json:"path"`
	Method        string                    `json:"method"`
	FixedSelector string                    `json:"fixed_selector,omitempty"`
	Sections      []reviewedProviderSection `json:"sections"`
}

func projectedProviderRequest(request providerRequest) reviewedProviderRequest {
	out := reviewedProviderRequest{Path: request.path, Method: request.method, Sections: []reviewedProviderSection{}}
	if out.Method == "" {
		out.Method = "GET"
	}
	if payload, ok := request.payload.(map[string]string); ok {
		out.FixedSelector = payload["ItemValue"]
	}
	for _, section := range request.sections {
		shape := "array"
		if section.object {
			shape = "object"
		}
		if section.keyed {
			shape = "array_or_keyed_object"
		}
		if section.root == "" {
			shape = "response_object"
			if request.responseArray {
				shape = "response_array"
			}
		}
		fields, units := []string{}, []string{}
		for _, field := range section.fields {
			fields = append(fields, field.key)
			units = append(units, field.unit)
		}
		out.Sections = append(out.Sections, reviewedProviderSection{section.root, shape, section.date, section.end, fields, units, section.optional, section.metadata})
		if (request.path == "/api/ihke3000/IHKE3212S01/page_load" || request.path == "/api/ihke3000/IHKE3212S02/page_load") && (section.root == "main_data" || section.root == "hist_list" || section.root == "treatData") {
			root := section.root
			if !section.object {
				root += "[]"
			}
			out.Sections = append(out.Sections, reviewedProviderSection{Root: root + ".refillList", Shape: "array", Fields: []string{"recipe_TIMES", "rel_DATE_STR", "recipe_MEMO", "hosp_NAME"}, Units: []string{"", "", "", ""}, Optional: true})
		}
	}
	return out
}

func projectedProviderDetails(category string) ([]reviewedProviderRequest, error) {
	details := []reviewedProviderRequest{}
	spec := providerSpecs[category]
	if category == "menstrual" {
		details = append(details, projectedProviderRequest(providerMenstrualMonthRequest(providerReference{rowID: "200001", sourceType: "200002", kind: "menstrual_month"})))
	}
	if category == "calendar_events" || category == "stored_reminders" {
		kinds := []string{"calendar_day"}
		if category == "stored_reminders" {
			kinds = []string{"stored_reminder", "custom_reminder", "chronic_reminder"}
		}
		for _, kind := range kinds {
			request, err := communicationsProviderDetailRequest(category, providerReference{kind: kind, rowID: "2000-01-01"})
			if err != nil {
				return nil, err
			}
			details = append(details, projectedProviderRequest(request))
		}
		return details, nil
	}
	if category == "recent_visits" {
		for _, request := range providerRecentDetailRequests() {
			details = append(details, projectedProviderRequest(request))
		}
		return details, nil
	}
	if category == "annual_overview" {
		for _, request := range annualProviderRequests(providerReference{rowID: "2000", kind: "annual_year"}) {
			details = append(details, projectedProviderRequest(request))
		}
		return details, nil
	}
	if spec.info.HasDetails || category == "glucose" {
		refs := []providerReference{{rowID: "0", sourceType: "A", query: Query{StartDate: "1912-01-01", EndDate: "2000-01-01"}}}
		if category == "physiology" || category == "device_activity" {
			details = append(details, projectedProviderRequest(providerPhysiologyRawRequest(category, refs[0])))
			refs = append(refs, providerReference{rowID: "10568", query: Query{StartDate: "1912-01-01", EndDate: "2000-01-01"}}, providerReference{rowID: "105", sourceType: "20000101", kind: "device_day"})
		}
		for _, selector := range providerCareCharts[category].selectors {
			refs = append(refs, providerReference{rowID: "synthetic-case", sourceType: selector.value, kind: "care_chart"})
		}
		for _, ref := range refs {
			request, err := providerDetailRequest(category, ref)
			if err != nil {
				return nil, err
			}
			details = append(details, projectedProviderRequest(request))
		}
	}
	return details, nil
}

func TestProviderRegistryPinsEveryImplementedProjectionAndSourceUnit(t *testing.T) {
	data, err := os.ReadFile("provider-contracts.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := strictjson.Validate(data, strictjson.Options{MaxDepth: 32, FoldNames: true}); err != nil {
		t.Fatal(err)
	}
	var registry struct {
		Version   string `json:"contract_revision"`
		Contracts []struct {
			ID         string                    `json:"id"`
			RangeMode  string                    `json:"range_mode"`
			HasDetails bool                      `json:"has_details"`
			List       []reviewedProviderRequest `json:"list"`
			Details    []reviewedProviderRequest `json:"details"`
			Assets     []string                  `json:"source_assets"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	if registry.Version != ProviderContractRevision {
		t.Fatal("response metadata and reviewed contract version differ")
	}
	seen := map[string]bool{}
	for _, contract := range registry.Contracts {
		spec, ok := providerSpecs[contract.ID]
		if !ok || seen[contract.ID] || len(contract.Assets) == 0 || contract.RangeMode != spec.info.RangeMode || contract.HasDetails != spec.info.HasDetails {
			t.Fatalf("unreviewed or stale category contract: %s", contract.ID)
		}
		seen[contract.ID] = true
		list := []reviewedProviderRequest{}
		for _, request := range spec.requests {
			list = append(list, projectedProviderRequest(request))
		}
		if !reflect.DeepEqual(list, contract.List) {
			t.Fatalf("list field/date/unit projection changed without review: %s", contract.ID)
		}
		details, err := projectedProviderDetails(contract.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(details, contract.Details) {
			t.Fatalf("detail field/date/unit projection changed without review: %s", contract.ID)
		}
	}
	if len(seen) != len(providerSpecs) {
		t.Fatal("provider category has no reviewed contract")
	}
}
