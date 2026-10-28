// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package lab

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// interval is an exact rational interval. nil represents an unbounded end.
// Inequalities and ranges never become fabricated point observations.
type interval struct {
	low, high       *big.Rat
	lowInc, highInc bool
}
type parsedValue struct {
	kind  string
	span  interval
	exact *big.Rat
	text  string
}

const decimalPattern = `[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d{1,2})?`

var exactPattern = regexp.MustCompile("^" + decimalPattern + "$")
var boundPattern = regexp.MustCompile(`^(>=|<=|>|<|≥|≤|大於等於|小於等於|大於|小於)\s*(` + decimalPattern + `)$`)
var rangePattern = regexp.MustCompile(`^(` + decimalPattern + `)\s*(?:-|–|~|～|至)\s*(` + decimalPattern + `)$`)
var ordinalPattern = regexp.MustCompile(`^(?:[1-4]\+|\+{1,4}|±|trace)$`)
var comparisonGlyphs = strings.NewReplacer("＜", "<", "＞", ">", "＝", "=", "≦", "≤", "≧", "≥")

func numericText(raw string) string {
	return comparisonGlyphs.Replace(strings.TrimSpace(raw))
}

func decimal(s string) (*big.Rat, bool) {
	s = strings.TrimSpace(s)
	if len(s) > 96 || !exactPattern.MatchString(s) {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(s)
	return r, ok
}
func parseValue(raw string) parsedValue {
	s := numericText(raw)
	p := parsedValue{text: strings.TrimSpace(raw), kind: "text"}
	if s == "" || s == "-" || s == "--" || s == "—" {
		p.kind = "missing"
		return p
	}
	if x, ok := decimal(s); ok {
		p.kind = "numeric"
		p.exact = x
		p.span = interval{x, x, true, true}
		return p
	}
	if m := boundPattern.FindStringSubmatch(s); m != nil {
		n, ok := decimal(m[2])
		if !ok {
			return p
		}
		p.kind = "inequality"
		switch m[1] {
		case ">", "大於":
			p.span = interval{low: n}
		case ">=", "≥", "大於等於":
			p.span = interval{low: n, lowInc: true}
		case "<", "小於":
			p.span = interval{high: n}
		default:
			p.span = interval{high: n, highInc: true}
		}
		return p
	}
	if m := rangePattern.FindStringSubmatch(s); m != nil {
		a, ok := decimal(m[1])
		b, ok2 := decimal(m[2])
		if ok && ok2 && a.Cmp(b) <= 0 {
			p.kind = "range"
			p.span = interval{a, b, true, true}
		} else {
			p.kind = "invalid"
		}
		return p
	}
	if ordinalPattern.MatchString(strings.ToLower(s)) {
		p.kind = "ordinal"
		return p
	}
	switch strings.ToLower(s) {
	case "negative", "positive", "normal", "not found", "none", "clear", "yellow", "no", "yes":
		p.kind = "qualitative"
	}
	return p
}
func (v interval) within(b interval) bool {
	if b.low != nil {
		if v.low == nil {
			return false
		}
		c := v.low.Cmp(b.low)
		if c < 0 || (c == 0 && v.lowInc && !b.lowInc) {
			return false
		}
	}
	if b.high != nil {
		if v.high == nil {
			return false
		}
		c := v.high.Cmp(b.high)
		if c > 0 || (c == 0 && v.highInc && !b.highInc) {
			return false
		}
	}
	return true
}
func (v interval) intersects(b interval) bool {
	if v.high != nil && b.low != nil {
		c := v.high.Cmp(b.low)
		if c < 0 || (c == 0 && !(v.highInc && b.lowInc)) {
			return false
		}
	}
	if b.high != nil && v.low != nil {
		c := b.high.Cmp(v.low)
		if c < 0 || (c == 0 && !(b.highInc && v.lowInc)) {
			return false
		}
	}
	return true
}
func textBound(s *string) *big.Rat {
	if s == nil {
		return nil
	}
	r, _ := decimal(*s)
	return r
}
func bandInterval(b Band) interval {
	return interval{textBound(b.Lower), textBound(b.Upper), b.LowerInclusive, b.UpperInclusive}
}
func pointer[T any](v T) *T { return &v }
func numberFloat(r *big.Rat) *float64 {
	if r == nil {
		return nil
	}
	v, _ := r.Float64()
	return &v
}
func numberString(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// DefinitelyAbove reports only an exact or bounded result wholly above the
// threshold. It is used for contextual warnings, never to invent a point.
func DefinitelyAbove(raw, threshold string) bool {
	p := parseValue(raw)
	limit, ok := decimal(threshold)
	if !ok || !numericKind(p) || p.span.low == nil {
		return false
	}
	c := p.span.low.Cmp(limit)
	return c > 0 || (c == 0 && !p.span.lowInc)
}

// DefinitelyAtLeast checks the lower bound without rounding or inventing a point.
func DefinitelyAtLeast(raw, threshold string) bool {
	p := parseValue(raw)
	limit, ok := decimal(threshold)
	return ok && numericKind(p) && p.span.low != nil && p.span.low.Cmp(limit) >= 0
}
