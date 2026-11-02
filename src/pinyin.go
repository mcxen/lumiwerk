package main

import (
	"strings"

	"github.com/mozillazg/go-pinyin"
)

// pinyinArgs reuses one conversion config; a is immutable after creation.
var pinyinArgs = pinyin.NewArgs()

// pinyinForms returns lowercase text plus every generated pinyin spelling:
// the concatenated full pinyin and the first-letter initialism.
func pinyinForms(s string) []string {
	out := []string{strings.ToLower(s)}
	result := pinyin.Pinyin(s, pinyinArgs)
	if len(result) == 0 {
		return out
	}
	var full, initials strings.Builder
	for _, py := range result {
		if len(py) == 0 {
			continue
		}
		full.WriteString(py[0])
		initials.WriteString(py[0][:1])
	}
	return append(out, full.String(), initials.String())
}

func pinyinContains(hay, needle string) bool {
	n := strings.ToLower(strings.TrimSpace(needle))
	if n == "" {
		return true
	}
	for _, form := range pinyinForms(hay) {
		if strings.Contains(form, n) {
			return true
		}
	}
	return false
}
