// Copyright (c) 2026 bestkakkoi llc. SPDX-License-Identifier: MIT
package lab

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var referenceBrackets = regexp.MustCompile(`\[([^\]]*)\]`)

func parseReference(raw string) (*string, *string) {
	m := referenceBrackets.FindAllStringSubmatch(raw, -1)
	if len(m) >= 2 {
		var a, b *string
		if _, ok := decimal(m[0][1]); ok {
			a = pointer(strings.TrimSpace(m[0][1]))
		}
		if _, ok := decimal(m[1][1]); ok {
			b = pointer(strings.TrimSpace(m[1][1]))
		}
		if a != nil && b != nil && textBound(a).Cmp(textBound(b)) > 0 {
			return nil, nil
		}
		return a, b
	}
	v := parseValue(raw)
	if v.kind == "range" {
		m := rangePattern.FindStringSubmatch(strings.TrimSpace(raw))
		return pointer(m[1]), pointer(m[2])
	}
	// Only inclusive unambiguous one-sided references are exposed as numeric
	// limits; strict inequality text remains visible rather than broadened.
	if v.kind == "inequality" {
		m := boundPattern.FindStringSubmatch(strings.TrimSpace(raw))
		if v.span.low != nil && v.span.lowInc {
			return pointer(m[2]), nil
		}
		if v.span.high != nil && v.span.highInc {
			return nil, pointer(m[2])
		}
	}
	return nil, nil
}
func numericKind(p parsedValue) bool {
	return p.kind == "numeric" || p.kind == "inequality" || p.kind == "range"
}
func referenceStatus(p parsedValue, low, high *string) string {
	if !numericKind(p) || (low == nil && high == nil) {
		return "unknown"
	}
	b := interval{textBound(low), textBound(high), true, true}
	if p.span.within(b) {
		return "within"
	}
	if b.low != nil && p.span.high != nil {
		c := p.span.high.Cmp(b.low)
		if c < 0 || (c == 0 && !p.span.highInc) {
			return "low"
		}
	}
	if b.high != nil && p.span.low != nil {
		c := p.span.low.Cmp(b.high)
		if c > 0 || (c == 0 && !p.span.lowInc) {
			return "high"
		}
	}
	return "unknown"
}
func validDomain(d Definition, p *parsedValue) bool {
	if !numericKind(*p) {
		return p.kind != "invalid"
	}
	min := textBound(d.Domain.Minimum)
	max := textBound(d.Domain.Maximum)
	domain := interval{min, max, true, true}
	if p.exact != nil {
		return p.span.within(domain)
	}
	if !p.span.intersects(domain) {
		return false
	}
	// Restrict an unbounded inequality to the physical domain. Do not clamp
	// explicit out-of-domain limits: malformed intervals must be investigated.
	if min != nil && p.span.low != nil && p.span.low.Cmp(min) < 0 {
		return false
	}
	if max != nil && p.span.high != nil && p.span.high.Cmp(max) > 0 {
		return false
	}
	if p.span.low == nil && min != nil {
		p.span.low = min
		p.span.lowInc = true
	}
	if p.span.high == nil && max != nil {
		p.span.high = max
		p.span.highInc = true
	}
	return true
}
func classifyBands(p parsedValue, bands []Band) (*int, bool) {
	if !numericKind(p) {
		return nil, false
	}
	for _, b := range bands {
		if p.span.within(bandInterval(b)) {
			return pointer(b.Level), true
		}
	}
	return nil, false
}

// guaranteedAlertFloor finds the least alert level that applies to every
// possible value. Adjacent source bands may jointly establish that floor even
// when an inequality/range cannot be assigned a single source category.
// It never turns an interval into a point or bridges an uncovered endpoint.
func guaranteedAlertFloor(p parsedValue, bands []Band) (*int, bool) {
	if !numericKind(p) {
		return nil, false
	}
	for level := 5; level >= 4; level-- {
		covered := make([]interval, 0, len(bands))
		for _, band := range bands {
			if band.Level >= level {
				covered = append(covered, bandInterval(band))
			}
		}
		sort.Slice(covered, func(i, j int) bool {
			if covered[i].low == nil {
				return covered[j].low != nil
			}
			if covered[j].low == nil {
				return false
			}
			comparison := covered[i].low.Cmp(covered[j].low)
			return comparison < 0 || (comparison == 0 && covered[i].lowInc && !covered[j].lowInc)
		})
		for i := 0; i < len(covered); {
			joined := covered[i]
			i++
			for i < len(covered) {
				next := covered[i]
				if joined.high != nil && next.low != nil {
					comparison := joined.high.Cmp(next.low)
					if comparison < 0 || (comparison == 0 && !joined.highInc && !next.lowInc) {
						break
					}
				}
				if joined.high != nil {
					if next.high == nil || next.high.Cmp(joined.high) > 0 {
						joined.high, joined.highInc = next.high, next.highInc
					} else if next.high.Cmp(joined.high) == 0 {
						joined.highInc = joined.highInc || next.highInc
					}
				}
				i++
			}
			if p.span.within(joined) {
				return pointer(level), true
			}
		}
	}
	return nil, false
}

func nonexactAlertExplanation(p parsedValue, bands []Band, level int) string {
	if p.kind == "numeric" {
		return ""
	}
	prefix := " 原始結果為不等式或區間，未換算為單點數值；"
	if _, singleCategory := classifyBands(p, bands); !singleCategory {
		prefix += "跨越來源分類界限，但"
	}
	return prefix + fmt.Sprintf("全部可能數值至少達本應用 L%d 警示。", level)
}

func sourceBands(r Rule) []Band {
	b := make([]Band, 0, len(r.Bands))
	for _, v := range r.Bands {
		v.Band.Level = v.DisplayLevel
		b = append(b, v.Band)
	}
	return b
}
func ruleMatchesUnit(rule Rule, unit string) bool {
	if unitKey(unit) == unitKey(rule.Unit) {
		return true
	}
	if rule.Unit == "mL/min/1.73m2" && unitKey(unit) == "ml/min/1.73m^2" {
		return true
	}
	for _, a := range rule.Aliases {
		if unitKey(a) == unitKey(unit) {
			return true
		}
	}
	return false
}
func (c *Catalog) Normalize(raw []RawRecord, s *Settings) []Record {
	out := make([]Record, 0, len(raw))
	if s.Known == nil {
		s.Known = map[string]KnownItem{}
	}
	for idx, r := range raw {
		d := c.match(r)
		p := parseValue(r.Result)
		lo, hi := parseReference(r.Reference)
		rec := Record{ID: rowID(r, idx), ItemID: d.ID, ConfigurationKey: d.Key, ItemName: r.ItemName, DisplayName: d.Name, Category: category(d), ExamDate: NormalizeDate(r.CheckDate), VisitDate: NormalizeDate(r.VisitDate), OrderCode: r.OrderCode, OrderName: r.OrderName, Result: r.Result, Unit: r.Unit, ReferenceLow: lo, ReferenceHigh: hi, ReferenceText: r.Reference, Institution: r.Institution, Source: r.Source, ReferenceStatus: referenceStatus(p, lo, hi), NumericValue: numberFloat(p.exact), ValueKind: p.kind, RuleSource: "none", OfficialFlag: r.OfficialFlag}
		if d.Kind == "metadata" {
			rec.ValueKind = "metadata"
			rec.NumericValue = nil
			rec.Interpretation = "檢驗流程／項目附註，不屬於可分級的測量值。"
			out = append(out, rec)
			continue
		}
		if p.kind != "missing" {
			s.Known[d.ID] = KnownItem{ID: d.ID, Key: d.Key, Name: d.Name, Unit: r.Unit, Category: rec.Category}
		}
		if !validDomain(d, &p) {
			rec.Level = nil
			rec.NumericValue = nil
			rec.ValueKind = "invalid"
			rec.Interpretation = "結果超出可接受的物理範圍，請核對原始報告；不作自動分級。"
			out = append(out, rec)
			continue
		}
		rec.Interpretation = "未提供可適用的數值基準，保留原始結果。"
		if p.kind == "missing" {
			rec.Interpretation = "原始報告未提供結果；缺值不視為0。"
		}
		switch rec.ReferenceStatus {
		case "within":
			rec.Level = pointer(1)
			rec.RuleSource = "live_report"
			rec.Interpretation = "位於本筆報告的原院參考區間。"
		case "high", "low":
			rec.Level = pointer(3)
			rec.RuleSource = "live_report"
			rec.Interpretation = "偏離本筆報告的原院參考區間；顏色不代表已校準的疾病嚴重度。"
		}
		rule := c.Rules[d.Rule]
		if rule.Kind == "numeric_bands" {
			if ruleMatchesUnit(rule, r.Unit) && numericKind(p) {
				if level, ok := classifyBands(p, sourceBands(rule)); ok {
					rec.Level = level
					rec.RuleSource = strings.Join(rule.Sources, ",")
					rec.Interpretation = description(d)
					for _, b := range rule.Bands {
						if p.span.within(bandInterval(b.Band)) {
							rec.Interpretation += " 分類：" + clinicalCategoryLabel(b.Category) + "。"
							break
						}
					}
				} else {
					rec.Level = nil
					rec.RuleSource = strings.Join(rule.Sources, ",")
					rec.Interpretation = "原始結果的範圍跨越分類界限，無法指定單一級別；" + description(d)
				}
			} else if !ruleMatchesUnit(rule, r.Unit) {
				rec.Interpretation = "單位尚未確認，未套用分類門檻；" + description(d)
			}
		}
		if d.Rule == "qualitative_reference" {
			token := strings.ToLower(strings.TrimSpace(r.Result))
			switch token {
			case "negative", "not found", "normal", "none":
				rec.Level = pointer(1)
				rec.RuleSource = "qualitative_reference"
				rec.Interpretation = "符合此項目的預期定性結果；不代表其他檢查均正常。"
			case "positive", "trace", "±", "1+", "2+", "3+", "4+", "+", "++", "+++", "++++":
				rec.Level = pointer(3)
				rec.RuleSource = "qualitative_reference"
				rec.Interpretation = "定性／半定量異常標記；加號數量不直接對應危險級別。"
			}
		}
		// Personal targets never replace the immutable source reference/flag fields.
		if o, ok := s.Overrides[d.Key]; ok && o.Enabled && numericKind(p) {
			if rule.Kind == "numeric_bands" && !ruleMatchesUnit(rule, r.Unit) {
				// In particular, a shared eGFR configuration must not apply its
				// verified-unit thresholds to a later unit-less source result.
				rec.Interpretation += " 個人設定尚未套用，須先確認此筆資料的量測單位。"
			} else if len(o.Bands) > 0 {
				if v, matched := classifyBands(p, o.Bands); matched {
					rec.Level = v
					rec.RuleSource = "personal"
					rec.Interpretation = "依個人設定的分級區間顯示；原院參考標記另行保留。"
				} else {
					rec.Level = nil
					rec.RuleSource = "personal"
					rec.Interpretation = "結果未完整落在單一個人設定區間；未指定單一分級。"
				}
			} else if o.ReferenceLow != nil || o.ReferenceHigh != nil {
				status := referenceStatus(p, o.ReferenceLow, o.ReferenceHigh)
				switch status {
				case "within":
					rec.Level = pointer(1)
				case "high", "low":
					rec.Level = pointer(3)
				default:
					rec.Level = nil
				}
				rec.RuleSource = "personal"
				rec.Interpretation = "依個人參考上下限顯示；原院參考標記另行保留。"
			}
		}
		// Independently applicable alert categories remain visible even after a
		// personal override. Screening diagnosis profiles remain disabled in v1.
		if d.ID == "fasting_plasma_glucose" && unitKey(r.Unit) == "mg/dl" {
			bands := sourceBands(c.Rules["blood_glucose_low_alerts"])
			if alert, ok := guaranteedAlertFloor(p, bands); ok {
				if rec.Level == nil || *rec.Level < *alert {
					rec.Level = alert
				}
				rec.RuleSource += ";ada_hypoglycemia"
				rec.Interpretation += " 血糖低於70 mg/dL的警示與是否空腹無關；ADA臨床第3級須依是否需他人協助判定。"
				rec.Interpretation += nonexactAlertExplanation(p, bands, *alert)
			}
		}
		if d.ID == "serum_triglycerides" && ruleMatchesUnit(rule, r.Unit) && numericKind(p) {
			bands := sourceBands(rule)
			if alert, ok := guaranteedAlertFloor(p, bands); ok {
				if rec.Level == nil || *rec.Level < *alert {
					rec.Level = alert
				}
				rec.RuleSource += ";" + strings.Join(rule.Sources, ",")
				rec.Interpretation += " 三酸甘油脂顯著升高需醫療評估；如合併腹痛或嘔吐應即時就醫。"
				rec.Interpretation += nonexactAlertExplanation(p, bands, *alert)
			}
		}
		if d.ID == "urine_glucose_dipstick" {
			rec.Interpretation += " SGLT2類用藥可增加尿糖。"
		}
		if d.ID == "venous_oxygen_saturation" || d.ID == "venous_po2" {
			rec.Interpretation += " 此為靜脈血氣，不能套用指尖SpO2／動脈血氧門檻。"
		}
		if rec.ExamDate == "" {
			rec.Interpretation += " 檢查日期格式待確認，未以就醫日期代替。"
		}
		out = append(out, rec)
	}
	return out
}
func ValidateSettings(s Settings, known map[string]KnownItem) error {
	if s.Theme != "light" && s.Theme != "dark" && s.Theme != "system" {
		return fmt.Errorf("外觀必須為light、dark或system")
	}
	if s.FontScale < 1 || s.FontScale > 2 {
		return fmt.Errorf("文字倍率須介於1與2")
	}
	if len(s.Overrides) > 500 || len(s.Favorites) > 500 || len(known) > 1000 {
		return fmt.Errorf("設定項目數量超出上限")
	}
	allowed := map[string]bool{}
	for _, v := range known {
		allowed[v.Key] = true
	}
	fav := map[string]bool{}
	for _, id := range s.Favorites {
		if !allowed[id] {
			return fmt.Errorf("只能收藏已出現的檢驗項目")
		}
		if fav[id] {
			return fmt.Errorf("收藏項目重複")
		}
		fav[id] = true
	}
	for key, o := range s.Overrides {
		if !allowed[key] {
			return fmt.Errorf("只能設定已出現的檢驗項目")
		}
		for _, v := range []*string{o.ReferenceLow, o.ReferenceHigh} {
			if v != nil {
				if _, ok := decimal(*v); !ok {
					return fmt.Errorf("參考值須為有效數字")
				}
			}
		}
		if o.ReferenceLow != nil && o.ReferenceHigh != nil && textBound(o.ReferenceLow).Cmp(textBound(o.ReferenceHigh)) > 0 {
			return fmt.Errorf("參考下限不可大於上限")
		}
		if len(o.Bands) > 16 {
			return fmt.Errorf("分級區間最多16個")
		}
		for i, b := range o.Bands {
			if b.Level < 1 || b.Level > 5 {
				return fmt.Errorf("級別須介於1至5")
			}
			for _, v := range []*string{b.Lower, b.Upper} {
				if v != nil {
					if _, ok := decimal(*v); !ok {
						return fmt.Errorf("區間界限須為有效數字")
					}
				}
			}
			x := bandInterval(b)
			if x.low != nil && x.high != nil {
				cmp := x.low.Cmp(x.high)
				if cmp > 0 || (cmp == 0 && !(x.lowInc && x.highInc)) {
					return fmt.Errorf("區間上下限不合法")
				}
			}
			for _, previous := range o.Bands[:i] {
				if x.intersects(bandInterval(previous)) {
					return fmt.Errorf("分級區間不可重疊，請檢查含等號的端點")
				}
			}
		}
	}
	return nil
}

// ValidateKnown protects portable configuration imports against arbitrary
// fields and unknown display definitions. No result data may be persisted.
func ValidateKnown(known map[string]KnownItem) error {
	for id, k := range known {
		if id != k.ID || len(id) > 100 || !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(id) || len(k.Key) > 100 || !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(k.Key) {
			return fmt.Errorf("invalid configuration identifier")
		}
		if len(k.Name) > 300 || len(k.Unit) > 80 || len(k.Category) > 80 {
			return fmt.Errorf("configuration definition too long")
		}
	}
	return nil
}
