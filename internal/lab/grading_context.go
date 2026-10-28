// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package lab

// setGradingContext replaces only the active base interpretation. Independent
// alert contexts are appended afterward, in the same order as alert evaluation.
// Scenario age/sex are intentionally absent: they are comparison inputs, not
// verified facts about the person or the collection represented by this row.
func setGradingContext(record *Record, kind, ruleID string, rule Rule) {
	record.GradingContexts = nil
	appendGradingContext(record, kind, ruleID, rule)
}

func appendGradingContext(record *Record, kind, ruleID string, rule Rule) {
	label, note := rule.ScopeLabel, rule.ApplicabilityNote
	if kind == "personal" {
		label = "個人顯示基準"
		note = "依使用者設定的顯示區間對照，未驗證為個人治療目標；原院結果、參考標記及獨立警示保留。"
	}
	if label == "" || note == "" {
		return
	}
	if kind == "independent_alert" {
		label += "・獨立提醒"
	}
	record.GradingContexts = append(record.GradingContexts, GradingContext{
		Kind: kind, RuleID: ruleID, Label: label, Note: note,
	})
}
