// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

// Package lab preserves source observations and applies explicitly scoped laboratory rules.
package lab

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

//go:embed catalog.json
var catalogJSON []byte

type match struct {
	Code string `json:"order_code"`
	Name string `json:"item_name"`
	Unit string `json:"unit_raw"`
}
type Definition struct {
	ID          string             `json:"id"`
	Name        string             `json:"display_name_zh_tw"`
	EnglishName string             `json:"display_name_en,omitempty"`
	TermNote    string             `json:"terminology_note,omitempty"`
	TermSources []string           `json:"terminology_sources,omitempty"`
	TermLoci    []terminologyLocus `json:"terminology_source_loci,omitempty"`
	TermBasis   string             `json:"terminology_basis,omitempty"`
	Match       match              `json:"source_match"`
	Aliases     []match            `json:"source_aliases,omitempty"`
	Key         string             `json:"configuration_key"`
	Kind        string             `json:"kind"`
	ValueKind   string             `json:"value_kind"`
	Rule        string             `json:"default_rule"`
	Alerts      []string           `json:"alert_rules,omitempty"`
	Reference   *ReferenceProfile  `json:"reference_profile"`
	Description string             `json:"description_zh_tw,omitempty"`
	Sources     []string           `json:"sources,omitempty"`
	Specimen    string             `json:"specimen"`
	Notes       []string           `json:"notes"`
	Domain      struct {
		Minimum *string `json:"minimum"`
		Maximum *string `json:"maximum"`
	} `json:"physical_domain"`
}

// ReferenceProfile is a named preview, separate from each report's immutable
// interval. Applicability is never inferred from coincident numeric limits.
type ReferenceProfile struct {
	Lower             *string  `json:"lower"`
	Upper             *string  `json:"upper"`
	LowerInclusive    bool     `json:"lower_inclusive"`
	UpperInclusive    bool     `json:"upper_inclusive"`
	Expected          []string `json:"expected_values,omitempty"`
	Source            string   `json:"source"`
	Scope             string   `json:"scope,omitempty"`
	AutomaticFallback bool     `json:"automatic_fallback_allowed"`
}
type ruleBand struct {
	Band
	DisplayLevel int    `json:"display_level"`
	Category     string `json:"category"`
}
type Rule struct {
	Kind              string     `json:"kind"`
	Unit              string     `json:"unit"`
	Aliases           []string   `json:"unit_aliases"`
	Bands             []ruleBand `json:"bands"`
	Sources           []string   `json:"sources"`
	Meaning           string     `json:"meaning"`
	Description       string     `json:"description,omitempty"`
	Conditions        []string   `json:"conditions,omitempty"`
	ScopeLabel        string     `json:"scope_label,omitempty"`
	ApplicabilityNote string     `json:"applicability_note,omitempty"`
	NormalText        []string   `json:"normal_text_values,omitempty"`
	PositiveText      []string   `json:"positive_text_values,omitempty"`
	ReviewText        []string   `json:"review_text_values,omitempty"`
}
type sourceDefinition struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	Edition       string `json:"edition"`
	Version       string `json:"version"`
	EffectiveDate string `json:"effective_date"`
}
type Catalog struct {
	ID           string                      `json:"catalog_id"`
	Items        []Definition                `json:"items"`
	Rules        map[string]Rule             `json:"rules"`
	Sources      map[string]sourceDefinition `json:"sources"`
	TermNotice   string                      `json:"terminology_notice"`
	NameGlossary []sourceNameTerm            `json:"source_name_glossary"`
}

func LoadCatalog() (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(catalogJSON, &c); err != nil {
		return nil, fmt.Errorf("decode embedded catalog: %w", err)
	}
	return &c, nil
}
func (c *Catalog) match(r RawRecord) Definition {
	for _, d := range c.Items {
		// Exact names preserve urine PH versus blood pH, even with identical units.
		if matchesSource(r, d.Match) {
			return d
		}
		for _, alias := range d.Aliases {
			if matchesSource(r, alias) {
				return d
			}
		}
		// eGFR is catalogued from a unit-less export. Explicit verified units may
		// select the same analyte, but the grading engine still rejects absent units.
		if d.ID == "egfr" && strings.EqualFold(r.ItemName, "eGFR") && r.OrderCode == d.Match.Code && unitKey(r.Unit) == "mL/min/1.73m2" {
			return d
		}
	}
	id := dynamicID(r.OrderCode, r.ItemName, r.Unit)
	return Definition{ID: id, Key: id, Name: r.ItemName, Match: match{r.OrderCode, r.ItemName, r.Unit}, Kind: "measurement", Rule: "report_reference"}
}
func matchesSource(r RawRecord, m match) bool {
	return strings.EqualFold(strings.TrimSpace(r.OrderCode), m.Code) && strings.TrimSpace(r.ItemName) == m.Name && unitKey(r.Unit) == unitKey(m.Unit)
}
func dynamicID(code, name, unit string) string {
	sum := sha256.Sum256([]byte(code + "\x00" + name + "\x00" + unitKey(unit)))
	return "custom_v2_" + hex.EncodeToString(sum[:10])
}
func category(d Definition) string {
	id := d.ID
	if d.Kind == "metadata" {
		return "metadata"
	}
	switch {
	case strings.Contains(id, "triglyceride") || strings.Contains(id, "cholesterol"):
		return "lipids"
	case id == "fasting_plasma_glucose" || id == "blood_hba1c" || id == "blood_glucose_unspecified" || id == "capillary_postprandial_glucose":
		return "glucose"
	case id == "blood_ck_mb_activity":
		return "cardiac"
	case id == "blood_total_homocysteine" || id == "serum_total_vitamin_b12":
		return "nutrition"
	case id == "plasma_d_dimer_unspecified":
		return "coagulation"
	case id == "blood_intact_pth":
		return "renal"
	case strings.HasPrefix(id, "venous_"):
		return "blood_gas"
	case strings.HasPrefix(id, "blood_"):
		return "hematology"
	case strings.Contains(id, "acr_") || id == "urine_pcr" || id == "serum_creatinine" || id == "serum_bun" || id == "egfr" || strings.Contains(id, "urine_albumin") || strings.HasPrefix(id, "spot_"):
		return "renal"
	case strings.HasPrefix(id, "urine_"):
		return "urinalysis"
	case id == "serum_alt" || id == "serum_albumin":
		return "liver"
	case id == "serum_potassium" || id == "serum_sodium" || id == "serum_chloride" || id == "serum_phosphorus" || id == "serum_total_calcium":
		return "electrolytes"
	case id == "plasma_hs_troponin_i" || id == "plasma_nt_probnp" || id == "serum_ck":
		return "cardiac"
	case id == "serum_crp":
		return "inflammation"
	default:
		return "other"
	}
}

type CatalogItem struct {
	ID          string            `json:"id"`
	Key         string            `json:"configuration_key"`
	Name        string            `json:"display_name"`
	EnglishName string            `json:"display_name_en,omitempty"`
	TermNote    string            `json:"terminology_note,omitempty"`
	TermSources []string          `json:"terminology_sources,omitempty"`
	Category    string            `json:"category"`
	Unit        string            `json:"unit"`
	Low         *string           `json:"reference_low"`
	High        *string           `json:"reference_high"`
	Rule        any               `json:"default_rule"`
	Description string            `json:"description"`
	Sources     []string          `json:"sources"`
	Builtin     bool              `json:"builtin"`
	Aliases     []string          `json:"aliases"`
	OrderCodes  []string          `json:"order_codes"`
	Reference   *ReferenceProfile `json:"default_reference"`
	Alerts      []Rule            `json:"alert_rules"`
	ValueKind   string            `json:"value_kind"`
}
type Source struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Edition string `json:"edition,omitempty"`
}
type CatalogResponse struct {
	Items        []CatalogItem `json:"items"`
	Sources      []Source      `json:"sources"`
	TermNotice   string        `json:"terminology_notice,omitempty"`
	NameGlossary []CatalogTerm `json:"source_name_glossary,omitempty"`
}

func (c *Catalog) ForSettings(s Settings) CatalogResponse {
	out := CatalogResponse{Items: []CatalogItem{}, Sources: []Source{}, TermNotice: c.TermNotice}
	for _, term := range c.NameGlossary {
		out.NameGlossary = append(out.NameGlossary, CatalogTerm{
			Names: append([]string(nil), term.Names...), Name: term.Name,
			EnglishName: term.EnglishName, Note: term.Note,
			Sources: append([]string(nil), term.Sources...), Basis: term.Basis,
		})
	}
	known := c.ConfigurableItems(s.Known)
	ids := make([]string, 0, len(known))
	for id := range known {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	seen := map[string]bool{}
	for _, id := range ids {
		k := known[id]
		if seen[k.Key] {
			continue
		}
		seen[k.Key] = true
		item := CatalogItem{ID: k.ID, Key: k.Key, Name: k.Name, Category: k.Category, Unit: k.Unit, Sources: []string{}, Aliases: []string{k.Name}, OrderCodes: []string{}, Alerts: []Rule{}, Description: "依本次檢驗的原院參考值顯示；個人設定只改變顯示基準。"}
		if k.LegacyFor == unitReviewRequired {
			item.Description = "舊版曾忽略單位大小寫，這份個人設定已停用並隔離保留，不套用新版紀錄。請核對原始單位後，在目前項目重新設定；數值及其他外觀偏好未被刪除。"
		} else if k.LegacyFor != "" {
			item.Description = "此舊版名稱曾有不同的個人規則，為避免覆蓋而獨立保留。新版結果使用對應的內建項目；還原此舊版設定後即可移除保留項目。"
		}
		for _, d := range c.Items {
			if d.Key == k.Key && d.Kind == "measurement" {
				item.Builtin = true
				item.EnglishName = d.EnglishName
				item.TermNote = d.TermNote
				item.TermSources = appendUnique(item.TermSources, d.TermSources...)
				item.ValueKind = d.ValueKind
				rule := c.Rules[d.Rule]
				item.Rule = rule
				item.Sources = appendUnique(item.Sources, rule.Sources...)
				item.Sources = appendUnique(item.Sources, d.Sources...)
				item.Aliases = appendUnique(item.Aliases, d.Match.Name)
				item.Aliases = appendUnique(item.Aliases, d.Name, d.EnglishName)
				item.OrderCodes = appendUnique(item.OrderCodes, d.Match.Code)
				for _, alias := range d.Aliases {
					item.Aliases = appendUnique(item.Aliases, alias.Name)
					item.OrderCodes = appendUnique(item.OrderCodes, alias.Code)
				}
				if item.Reference == nil && d.Reference != nil {
					item.Reference = d.Reference
					item.Sources = appendUnique(item.Sources, d.Reference.Source)
				}
				if len(item.Alerts) == 0 {
					for _, id := range d.Alerts {
						alert := c.Rules[id]
						item.Alerts = append(item.Alerts, alert)
						item.Sources = appendUnique(item.Sources, alert.Sources...)
					}
				}
				item.Description = description(d)
			}
		}
		if !item.Builtin && k.LegacyFor == "" {
			c.describeSourceName(&item, k.Name)
		}
		out.Items = append(out.Items, item)
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].ID < out.Items[j].ID })
	for id, s := range c.Sources {
		edition := s.Edition
		if edition == "" {
			edition = s.Version
		}
		out.Sources = append(out.Sources, Source{id, s.Title, s.URL, edition})
	}
	sort.Slice(out.Sources, func(i, j int) bool { return out.Sources[i].ID < out.Sources[j].ID })
	return out
}

// ConfigurableItems exposes built-ins without claiming they were observed or
// persisting a copy of the catalog in every user's SQL settings document.
func (c *Catalog) ConfigurableItems(known map[string]KnownItem) map[string]KnownItem {
	out := make(map[string]KnownItem, len(known)+len(c.Items))
	for id, item := range known {
		out[id] = item
	}
	for _, d := range c.Items {
		if d.Kind != "measurement" || d.Key == "" {
			continue
		}
		unit := d.Match.Unit
		if rule := c.Rules[d.Rule]; rule.Unit != "" {
			unit = rule.Unit
		}
		out[d.ID] = KnownItem{ID: d.ID, Key: d.Key, Name: d.Name, Unit: unit, Category: category(d)}
	}
	return out
}

// ReconcileSettings promotes formerly dynamic identities now recognized by a
// verified source match. Conflicting personal edits remain explicitly labelled
// instead of being discarded; resets can subsequently remove the conflict.
func (c *Catalog) ReconcileSettings(s *Settings) {
	if s.Known == nil {
		s.Known = map[string]KnownItem{}
	}
	if s.Overrides == nil {
		s.Overrides = map[string]Override{}
	}
	c.isolateAmbiguousUnitSettings(s)
	for _, d := range c.Items {
		if d.Kind != "measurement" || d.Key == "" {
			continue
		}
		matches := append([]match{d.Match}, d.Aliases...)
		for _, m := range matches {
			oldID := dynamicID(m.Code, m.Name, m.Unit)
			old, ok := s.Known[oldID]
			if !ok || old.ID != oldID || old.Key != oldID || unitKey(old.Unit) != unitKey(m.Unit) {
				continue
			}
			previous, hasPrevious := s.Overrides[oldID]
			current, hasCurrent := s.Overrides[d.Key]
			if old.LegacyFor == d.Key && hasPrevious {
				continue
			}
			if hasPrevious && hasCurrent && !reflect.DeepEqual(previous, current) {
				old.Name = m.Name + "（保留的舊版設定）"
				old.LegacyFor = d.Key
				s.Known[oldID] = old
				continue
			}
			if hasPrevious {
				s.Overrides[d.Key] = previous
				delete(s.Overrides, oldID)
			}
			for i, key := range s.Favorites {
				if key == oldID {
					s.Favorites[i] = d.Key
				}
			}
			s.Favorites = appendUnique([]string{}, s.Favorites...)
			delete(s.Known, oldID)
			s.Known[d.ID] = KnownItem{ID: d.ID, Key: d.Key, Name: d.Name, Unit: old.Unit, Category: category(d)}
		}
	}
}

func appendUnique(values []string, additions ...string) []string {
	for _, value := range additions {
		if value == "" {
			continue
		}
		present := false
		for _, existing := range values {
			if existing == value {
				present = true
				break
			}
		}
		if !present {
			values = append(values, value)
		}
	}
	return values
}

func description(d Definition) string {
	if d.Description != "" {
		return d.Description
	}
	switch d.ID {
	case "egfr":
		return "KDIGO G1–G5（G3a／G3b分開標示）；須確認單位，單次結果無法判定慢性病程，亦須合看白蛋白尿。"
	case "urine_acr_quantitative", "urine_acr_semiquantitative":
		return "KDIGO A1 <30、A2 30–300、A3 >300 mg/g。半定量「≥300」跨越兩級，保留原始界限。"
	case "serum_triglycerides":
		return "成人三酸甘油脂分界為150、200、500、1000 mg/dL；顏色為本應用的提醒分級，低階判讀須確認空腹。"
	case "urine_pcr":
		return "尿總蛋白／肌酸酐比值：參考KDIGO 2012的150與500 mg/g分界，並非白蛋白尿A分期。"
	case "blood_hba1c":
		return "原院參考區間與糖尿病診斷界值不同；個人治療目標應由醫療人員設定，低值也需保留異常旗標。"
	case "urine_glucose_dipstick":
		return "尿糖陽性只作異常標記；SGLT2類藥物會增加尿糖，不能用加號數量推定危險程度。"
	case "venous_oxygen_saturation", "venous_po2":
		return "血氣檢體的動脈／靜脈來源未確認，不直接套用動脈血氧或指尖 SpO2 標準；保留原院區間與原始結果。"
	case "serum_ldl_cholesterol":
		return "個人治療目標依心血管風險而異；三酸甘油脂極高時須確認是直接量測或計算值。"
	default:
		return "優先使用每筆報告的原院參考值。無適用來源時顯示未判定；不把偏離百分比換算為危險程度。"
	}
}

func clinicalCategoryLabel(value string) string {
	labels := map[string]string{
		"below150": "低於150 mg/dL", "borderline_high": "邊界偏高", "high": "偏高",
		"severe": "嚴重升高", "very_high_pancreatitis_attention": "非常高，需評估胰臟炎風險",
		"normal_to_mildly_increased_protein": "正常至輕度蛋白尿增加",
		"moderately_increased_protein":       "中度蛋白尿增加",
		"severely_increased_protein":         "重度蛋白尿增加",
		"vgh_critical_threshold":             "臺北榮總危急值門檻參考",
		"vgh_abnormal_notification":          "臺北榮總異常值通報門檻參考",
	}
	if label, ok := labels[value]; ok {
		return label
	}
	return value
}
