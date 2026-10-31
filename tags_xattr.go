package main

import (
	"encoding/xml"
	"os/exec"
	"strings"
)

const finderTagXattr = "com.apple.metadata:_kMDItemUserTags"

func writeTagXattr(path string, tags []string) error {
	data, err := binaryPlistStrings(tags)
	if err != nil {
		return err
	}
	cmd := exec.Command("xattr", "-w", "-x", finderTagXattr, hexEncode(data), path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) == 0 {
			return err
		}
		return err
	}
	return nil
}

func readTagXattr(path string) ([]string, error) {
	out, err := exec.Command("xattr", "-px", finderTagXattr, path).Output()
	if err != nil || len(out) == 0 {
		return nil, nil
	}
	raw, err := hexDecode(string(out))
	if err != nil || len(raw) == 0 {
		return nil, nil
	}
	cmd := exec.Command("plutil", "-convert", "xml1", "-o", "-", "-")
	cmd.Stdin = strings.NewReader(string(raw))
	xmlOut, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	rawTags := plistStrings(xmlOut)
	tags := make([]string, 0, len(rawTags))
	for _, tag := range rawTags {
		if i := strings.IndexByte(tag, '\n'); i >= 0 {
			tag = tag[:i]
		}
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags, nil
}

func binaryPlistStrings(tags []string) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`)
	b.WriteString(`<plist version="1.0"><array>`)
	for _, tag := range tags {
		b.WriteString("<string>")
		xml.EscapeText(&b, []byte(tag))
		b.WriteString("</string>")
	}
	b.WriteString(`</array></plist>`)
	cmd := exec.Command("plutil", "-convert", "binary1", "-o", "-", "-")
	cmd.Stdin = strings.NewReader(b.String())
	return cmd.Output()
}

func plistStrings(data []byte) []string {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var tags []string
	inString := false
	var cur strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch node := tok.(type) {
		case xml.StartElement:
			if node.Name.Local == "string" {
				inString = true
				cur.Reset()
			}
		case xml.EndElement:
			if node.Name.Local == "string" && inString {
				tags = append(tags, cur.String())
				inString = false
			}
		case xml.CharData:
			if inString {
				cur.Write(node)
			}
		}
	}
	return tags
}

func hexEncode(data []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(data)*2)
	for i, b := range data {
		out[i*2] = digits[b>>4]
		out[i*2+1] = digits[b&0x0f]
	}
	return string(out)
}

func hexDecode(s string) ([]byte, error) {
	var digits []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		digits = append(digits, c)
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0')
	}
	out := make([]byte, len(digits)/2)
	for i := 0; i < len(out); i++ {
		hi, err := hexVal(digits[i*2])
		if err != nil {
			return nil, err
		}
		lo, err := hexVal(digits[i*2+1])
		if err != nil {
			return nil, err
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

func hexVal(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	default:
		return 0, errObjC
	}
}
