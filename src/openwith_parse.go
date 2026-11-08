package main

import (
	"regexp"
	"strings"
)

var regValueLine = regexp.MustCompile(`^\s*([A-Za-z]|\(Default\)|MRUList)\s+REG_\w+\s+(.*)$`)

// parseOpenWithList extracts program names from `reg query` output of an
// OpenWithList key (values a, b, c… hold executable names; MRUList is order).
func parseOpenWithList(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		m := regValueLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil || m[1] == "MRUList" || m[1] == "(Default)" {
			continue
		}
		if v := strings.TrimSpace(m[2]); v != "" {
			names = append(names, v)
		}
	}
	return names
}

// commandExecutable pulls the program path out of a `reg query ... /ve`
// result such as `(Default)  REG_SZ  "C:\Apps\x.exe" "%1"`.
func commandExecutable(out string) string {
	for _, line := range strings.Split(out, "\n") {
		m := regValueLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil || m[1] != "(Default)" {
			continue
		}
		cmd := strings.TrimSpace(m[2])
		if strings.HasPrefix(cmd, `"`) {
			if end := strings.Index(cmd[1:], `"`); end >= 0 {
				return cmd[1 : 1+end]
			}
			return ""
		}
		if i := strings.Index(strings.ToLower(cmd), ".exe"); i >= 0 {
			return cmd[:i+4]
		}
	}
	return ""
}
