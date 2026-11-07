package main

import (
	"math"
	"strings"
	"testing"
)

func TestTokeniserClassifiesTokens(t *testing.T) {
	var tokens TTokeniser
	tokens.Initialise("sensor.wind_speed!x 'a b' \"c\"\n  ${ -5 \"open")
	want := []TToken{
		{Kind: TokWord, Value: "sensor", Line: 1, Column: 0},
		{Kind: TokPunctuation, Value: ".", Line: 1, Column: 6},
		{Kind: TokWord, Value: "wind_speed", Line: 1, Column: 7},
		{Kind: TokPunctuation, Value: "!", Line: 1, Column: 17},
		{Kind: TokWord, Value: "x", Line: 1, Column: 18},
		{Kind: TokString, Value: "a b", PrecededByWhitespace: true, Line: 1, Column: 20},
		{Kind: TokString, Value: "c", PrecededByWhitespace: true, Line: 1, Column: 26},
		{Kind: TokPunctuation, Value: "$", PrecededByWhitespace: true, Line: 2, Column: 2},
		{Kind: TokPunctuation, Value: "{", Line: 2, Column: 3},
		{Kind: TokWord, Value: "-5", PrecededByWhitespace: true, Line: 2, Column: 5},
		{Kind: TokUnterminated, Value: "open", PrecededByWhitespace: true, Line: 2, Column: 8},
		{Kind: TokEOF, Line: 2, Column: 13},
	}
	for i, w := range want {
		if got := tokens.ThisToken(); got != w {
			t.Fatalf("token %d: got %+v, want %+v", i, got, w)
		}
		tokens.NextToken()
	}
}

func TestReportParsingErrorReportsOnceAtTheToken(t *testing.T) {
	var p TDefParser
	p.Initialise("alpha beta; gamma", "test")
	if p.ForcedThisWordWas("alpha") && p.ForcedThisWordWas("delta") {
		t.Fatal("matched delta")
	}
	if !p.ThisWordIs("gamma") {
		t.Errorf("not skipped past the \";\": at %+v", p.ThisToken())
	}
	p.ForcedThisWordWas("epsilon")
	if len(p.Warnings) != 1 || p.Warnings[0] != `test: expected "delta", got "beta" (L:1, C:6)` {
		t.Errorf("warnings %q", p.Warnings)
	}
	if p.ForcedEndOfStatement() {
		t.Error("a statement with an error ended well")
	}
}

func TestForcedNumber(t *testing.T) {
	for source, want := range map[string]string{
		"7":  "",
		"x":  `"x" is not a count`,
		"0":  `"0" is not a count`,
		"10": `"10" is not a count`,
		";":  `expected a count, got ";"`,
	} {
		var p TDefParser
		var n int
		p.Initialise(source, "test")
		ok := p.ForcedNumber(&n, 1, 9, "a count")
		switch {
		case want == "" && (!ok || n != 7 || !p.AtEndOfTokens()):
			t.Errorf("%q: ok %v, n %d, %v", source, ok, n, p.Warnings)
		case want != "" && (ok || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], want)):
			t.Errorf("%q: ok %v, warnings %q, want %q", source, ok, p.Warnings, want)
		}
	}
	var p TDefParser
	var n int
	p.Initialise("90", "test")
	if !p.ForcedNumber(&n, 1, math.MaxInt, "days") || n != 90 {
		t.Errorf("no maximum: %d %v", n, p.Warnings)
	}
}

func TestRuleParserReportsTheWrongWord(t *testing.T) {
	for line, want := range map[string]string{
		"when light.social:x turn on":            `expected "turns", "changes" or "is", got "turn" (L:1, C:20)`,
		"then turn light.social:x of;":           `expected "on" or "off", got "of" (L:1, C:25)`,
		"if time aftr 10:00":                     `expected "after", "before" or "weekday", got "aftr" (L:1, C:8)`,
		"then frobnicate light.social:x;":        `got "frobnicate" (L:1, C:5)`,
		"when light.social:x turns on for 00:05": `expected ":", got the end of the statement (L:1, C:38)`,
		"then dim light.social:x with 150%;":     `"150" is not the step in brightness (0 to 100) (L:1, C:29)`,
	} {
		var triggers []TRuleTrigger
		var cond TRuleCondition
		var action TRuleAction
		p := newRuleParser(line, "r")
		if p.WhenStatement(&triggers) || p.IfStatement(&cond) || p.ThenStatement(&action) {
			t.Errorf("%q parsed", line)
		}
		if len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], want) {
			t.Errorf("%q: warnings %q, want %q", line, p.Warnings, want)
		}
	}
}

func TestRuleParserJoinsEntityPaths(t *testing.T) {
	for line, want := range map[string]string{
		"when light.social:hall/ceiling turns on": "light.social:hall/ceiling",
		"when vacuum.social: turns error":         "vacuum.social:",
		"when light.social:/hall turns on":        "light.social:/hall",
	} {
		var triggers []TRuleTrigger
		p := newRuleParser(line, "r")
		if !p.WhenStatement(&triggers) || triggers[0].EntitySpec != want {
			t.Errorf("%q: %+v %v, want %q", line, triggers, p.Warnings, want)
		}
	}
}

func TestConditionParserJoinsNamesAndArguments(t *testing.T) {
	templates := map[string]TJinjaTemplateDefinition{"int_less_than": {Params: []string{"n"}, Template: "$ < ${n}"}}
	tree, warnings := parseConditionExpr("jinja (sensor.x) ${int_less_than}(10.5)", templates, "test")
	if tree == nil || tree.JinjaExpr != "$1 < 10.5" || tree.JinjaSources[0] != "sensor.x" {
		t.Errorf("template: %+v %v", tree, warnings)
	}
	tree, warnings = parseConditionExpr("weather.forecast!pressure is available", nil, "test")
	if tree == nil || tree.Ref != "weather.forecast!pressure" || !tree.IsAvailable {
		t.Errorf("attribute: %+v %v", tree, warnings)
	}
	for raw, want := range map[string]string{
		"sensor.x is 'a":         `got a string without its closing quote (L:1, C:12)`,
		"sensor.x from":          `expected a device id, got the end of the statement (L:1, C:13)`,
		"sensor.x! is available": `expected the rest of the name, got "is" (L:1, C:10)`,
		"sensor. x":              `expected the rest of the name, got "x" (L:1, C:8)`,
	} {
		if tree, warnings := parseConditionExpr(raw, nil, "test"); tree != nil || len(warnings) != 1 || !strings.Contains(warnings[0], want) {
			t.Errorf("%q: %+v %q, want %q", raw, tree, warnings, want)
		}
	}
}
