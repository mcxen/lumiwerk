package main

import "strings"

type pinyinEntry struct {
	full    string
	initial string
}

// Real pinyin mapping for characters common in Chinese media filenames.
var pinyinMap = map[rune][]pinyinEntry{
	'图': {{"tu", "t"}}, '片': {{"pian", "p"}}, '视': {{"shi", "s"}}, '频': {{"pin", "p"}},
	'屏': {{"ping", "p"}}, '幕': {{"mu", "m"}}, '截': {{"jie", "j"}}, '照': {{"zhao", "z"}},
	'像': {{"xiang", "x"}}, '音': {{"yin", "y"}}, '文': {{"wen", "w"}}, '件': {{"jian", "j"}},
	'夹': {{"jia", "j"}}, '新': {{"xin", "x"}}, '建': {{"jian", "j"}}, '小': {{"xiao", "x"}},
	'猫': {{"mao", "m"}}, '二': {{"er", "e"}}, '期': {{"qi", "q"}}, '打': {{"da", "d"}},
	'包': {{"bao", "b"}}, '好': {{"hao", "h"}}, '的': {{"de", "d"}}, '地': {{"de", "d"}},
	'方': {{"fang", "f"}}, '分': {{"fen", "f"}}, '类': {{"lei", "l"}}, '结': {{"jie", "j"}},
	'果': {{"guo", "g"}}, '未': {{"wei", "w"}}, '命': {{"ming", "m"}}, '名': {{"ming", "m"}},
	'影': {{"ying", "y"}}, '集': {{"ji", "j"}}, '剧': {{"ju", "j"}},
}

// pinyinForms returns lowercase text plus every generated pinyin spelling.
func pinyinForms(s string) []string {
	out := []string{strings.ToLower(s)}
	// Full pinyin concatenation and initialism, ignoring unmatched runes.
	var full, initials strings.Builder
	hasCJK := false
	for _, r := range s {
		if entries, ok := pinyinMap[r]; ok {
			hasCJK = true
			full.WriteString(entries[0].full)
			initials.WriteString(entries[0].initial)
		}
	}
	if hasCJK {
		out = append(out, full.String(), initials.String())
		// Also space-separated full pinyin, so partial queries match names.
		out = append(out, full.String()+" "+initials.String())
	}
	return out
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
