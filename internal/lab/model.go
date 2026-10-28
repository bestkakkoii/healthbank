// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package lab

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// RawRecord contains only the laboratory row, never the patient banner.
type RawRecord struct {
	CheckDate    string
	OrderCode    string
	OrderName    string
	ItemName     string
	Result       string
	Unit         string
	Reference    string
	Institution  string
	VisitDate    string
	Source       string
	OfficialFlag string
}

type Record struct {
	ID               string   `json:"id"`
	ItemID           string   `json:"item_id"`
	ConfigurationKey string   `json:"configuration_key"`
	ItemName         string   `json:"item_name"`
	DisplayName      string   `json:"display_name"`
	Category         string   `json:"category"`
	ExamDate         string   `json:"exam_date"`
	VisitDate        string   `json:"visit_date"`
	RawCheckDate     string   `json:"raw_check_date"`
	RawVisitDate     string   `json:"raw_visit_date"`
	OrderCode        string   `json:"order_code"`
	OrderName        string   `json:"order_name"`
	Result           string   `json:"result"`
	Unit             string   `json:"unit"`
	ReferenceLow     *string  `json:"reference_low"`
	ReferenceHigh    *string  `json:"reference_high"`
	ReferenceText    string   `json:"reference_text"`
	Institution      string   `json:"institution"`
	Source           string   `json:"source"`
	Level            *int     `json:"level"`
	ReferenceStatus  string   `json:"reference_status"`
	Interpretation   string   `json:"interpretation"`
	NumericValue     *float64 `json:"numeric_value"`
	ValueKind        string   `json:"value_kind"`
	RuleSource       string   `json:"rule_source"`
	OfficialFlag     string   `json:"official_flag,omitempty"`
}

type Band struct {
	Lower          *string `json:"lower"`
	Upper          *string `json:"upper"`
	LowerInclusive bool    `json:"lower_inclusive"`
	UpperInclusive bool    `json:"upper_inclusive"`
	Level          int     `json:"level"`
}
type Override struct {
	ReferenceLow  *string `json:"reference_low,omitempty"`
	ReferenceHigh *string `json:"reference_high,omitempty"`
	Bands         []Band  `json:"bands,omitempty"`
	Enabled       bool    `json:"enabled"`
}

// AnalysisScenario contains explicit comparison settings, never a birth date or history.
type AnalysisScenario struct {
	Age *int   `json:"age"`
	Sex string `json:"sex"`
}
type AnalysisPreferences struct {
	Mode       string           `json:"mode"`
	Primary    AnalysisScenario `json:"primary"`
	Comparison AnalysisScenario `json:"comparison"`
}
type Settings struct {
	Analysis  *AnalysisPreferences `json:"analysis,omitempty"`
	Revision  int64                `json:"revision"`
	Theme     string               `json:"theme"`
	FontScale float64              `json:"font_scale"`
	Favorites []string             `json:"favorites"`
	Overrides map[string]Override  `json:"overrides"`
	// Known stores definitions for encountered items only; it contains no values,
	// dates, institution, patient identifiers, or result history.
	Known map[string]KnownItem `json:"known_items,omitempty"`
}
type KnownItem struct {
	ID       string `json:"id"`
	Key      string `json:"configuration_key"`
	Name     string `json:"name"`
	Unit     string `json:"unit"`
	Category string `json:"category"`
	// LegacyFor keeps a conflicting previous target independent until reset.
	LegacyFor string `json:"legacy_for,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{Theme: "system", FontScale: 1, Favorites: []string{}, Overrides: map[string]Override{}, Known: map[string]KnownItem{}}
}
func DecodeSettings(data []byte, revision int64) (Settings, error) {
	s := DefaultSettings()
	if len(data) > 0 {
		if err := json.Unmarshal(data, &s); err != nil {
			return s, fmt.Errorf("decode settings: %w", err)
		}
	}
	s.Revision = revision
	if s.Overrides == nil {
		s.Overrides = map[string]Override{}
	}
	if s.Known == nil {
		s.Known = map[string]KnownItem{}
	}
	if s.Favorites == nil {
		s.Favorites = []string{}
	}
	return s, nil
}
func (s Settings) Document() ([]byte, error) {
	s.Revision = 0
	return json.Marshal(s)
}

var datePattern = regexp.MustCompile(`^(\d{2,4})[-/.](\d{1,2})[-/.](\d{1,2})(?:[ T].*)?$`)

func NormalizeDate(raw string) string {
	m := datePattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return ""
	}
	var year, month, day int
	if _, err := fmt.Sscanf(strings.Join(m[1:], "-"), "%d-%d-%d", &year, &month, &day); err != nil {
		return ""
	}
	if year < 1911 {
		year += 1911
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day || year < 1912 || year > 2200 {
		return ""
	}
	return t.Format("2006-01-02")
}
func rowID(r RawRecord, index int) string {
	b, _ := json.Marshal(r)
	sum := sha256.Sum256(append(b, []byte(fmt.Sprintf("\x00%d", index))...))
	return hex.EncodeToString(sum[:12])
}
