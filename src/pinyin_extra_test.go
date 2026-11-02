package main

import "testing"

func TestPinyinFullDictionaryCoverage(t *testing.T) {
	// Characters outside the old hand-written map must now match.
	cases := []struct{ name, q string }{
		{"壁纸合集", "bizhi"},
		{"壁纸合集", "bzhj"},
		{"旅游照片2024", "lvyou"},
		{"旅游照片2024", "lyzp"},
		{"生日聚会", "shengri"},
		{"猫", "mao"},
	}
	for _, c := range cases {
		if !pinyinContains(c.name, c.q) {
			t.Errorf("pinyinContains(%q, %q) = false, want true", c.name, c.q)
		}
	}
	if pinyinContains("生日聚会", "xyz") {
		t.Errorf("pinyinContains matched a bogus query")
	}
}
