// Copyright (c) 2026 bestkakkoi llc. SPDX-License-Identifier: MIT
package lab

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	ID        string   `json:"id"`
	Name      string   `json:"display_name_zh_tw"`
	Match     match    `json:"source_match"`
	Key       string   `json:"configuration_key"`
	Kind      string   `json:"kind"`
	ValueKind string   `json:"value_kind"`
	Rule      string   `json:"default_rule"`
	Specimen  string   `json:"specimen"`
	Notes     []string `json:"notes"`
	Domain    struct {
		Minimum *string `json:"minimum"`
		Maximum *string `json:"maximum"`
	} `json:"physical_domain"`
}
type ruleBand struct {
	Band
	DisplayLevel int    `json:"display_level"`
	Category     string `json:"category"`
}
type Rule struct {
	Kind    string     `json:"kind"`
	Unit    string     `json:"unit"`
	Aliases []string   `json:"unit_aliases"`
	Bands   []ruleBand `json:"bands"`
	Sources []string   `json:"sources"`
	Meaning string     `json:"meaning"`
}
type sourceDefinition struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	Edition       string `json:"edition"`
	Version       string `json:"version"`
	EffectiveDate string `json:"effective_date"`
}
type Catalog struct {
	Items   []Definition                `json:"items"`
	Rules   map[string]Rule             `json:"rules"`
	Sources map[string]sourceDefinition `json:"sources"`
}

func LoadCatalog() (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(catalogJSON, &c); err != nil {
		return nil, fmt.Errorf("decode embedded catalog: %w", err)
	}
	return &c, nil
}
func unitKey(s string) string {
	s = strings.TrimSpace(s)
	r := strings.NewReplacer("μ", "u", "µ", "u", "²", "2", "³", "3", " ", "", "／", "/", "m²", "m2")
	return strings.ToLower(r.Replace(s))
}
func (c *Catalog) match(r RawRecord) Definition {
	for _, d := range c.Items {
		// Exact names preserve urine PH versus blood pH, even with identical units.
		if strings.EqualFold(strings.TrimSpace(r.OrderCode), d.Match.Code) && strings.TrimSpace(r.ItemName) == d.Match.Name && unitKey(r.Unit) == unitKey(d.Match.Unit) {
			return d
		}
		// eGFR is catalogued from a unit-less export. Explicit verified units may
		// select the same analyte, but the grading engine still rejects absent units.
		if d.ID == "egfr" && strings.EqualFold(r.ItemName, "eGFR") && r.OrderCode == d.Match.Code && (unitKey(r.Unit) == "ml/min/1.73m2" || unitKey(r.Unit) == "ml/min/1.73m^2") {
			return d
		}
	}
	sum := sha256.Sum256([]byte(r.OrderCode + "\x00" + r.ItemName + "\x00" + unitKey(r.Unit)))
	id := "custom_" + hex.EncodeToString(sum[:10])
	return Definition{ID: id, Key: id, Name: r.ItemName, Match: match{r.OrderCode, r.ItemName, r.Unit}, Kind: "measurement", Rule: "report_reference"}
}
func category(d Definition) string {
	id := d.ID
	if d.Kind == "metadata" {
		return "metadata"
	}
	switch {
	case strings.Contains(id, "triglyceride") || strings.Contains(id, "cholesterol"):
		return "lipids"
	case id == "fasting_plasma_glucose" || id == "blood_hba1c":
		return "glucose"
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
	ID          string   `json:"id"`
	Key         string   `json:"configuration_key"`
	Name        string   `json:"display_name"`
	Category    string   `json:"category"`
	Unit        string   `json:"unit"`
	Low         *string  `json:"reference_low"`
	High        *string  `json:"reference_high"`
	Rule        any      `json:"default_rule"`
	Description string   `json:"description"`
	Sources     []string `json:"sources"`
}
type Source struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Edition string `json:"edition,omitempty"`
}
type CatalogResponse struct {
	Items   []CatalogItem `json:"items"`
	Sources []Source      `json:"sources"`
}

func (c *Catalog) ForSettings(s Settings) CatalogResponse {
	out := CatalogResponse{Items: []CatalogItem{}, Sources: []Source{}}
	ids := make([]string, 0, len(s.Known))
	for id := range s.Known {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	seen := map[string]bool{}
	for _, id := range ids {
		k := s.Known[id]
		if seen[k.Key] {
			continue
		}
		seen[k.Key] = true
		item := CatalogItem{ID: k.ID, Key: k.Key, Name: k.Name, Category: k.Category, Unit: k.Unit, Sources: []string{}, Description: "依本次檢驗的原院參考值顯示；個人設定只改變顯示基準。"}
		for _, d := range c.Items {
			if d.ID == k.ID {
				rule := c.Rules[d.Rule]
				item.Rule = rule
				item.Sources = rule.Sources
				if item.Sources == nil {
					item.Sources = []string{}
				}
				item.Description = description(d)
				break
			}
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
func description(d Definition) string {
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
		return "靜脈血氣：不可套用動脈血氧或指尖SpO2標準；顯示原院區間與原始結果。"
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
	}
	if label, ok := labels[value]; ok {
		return label
	}
	return value
}
