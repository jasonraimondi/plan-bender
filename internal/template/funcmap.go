package template

import (
	"regexp"
	"strings"
	"text/template"
	"unicode"
)

// FuncMap returns the custom template functions.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"upper":  fnUpper,
		"lower":  fnLower,
		"kebab":  fnKebab,
		"join":   fnJoin,
		"indent": fnIndent,
	}
}

func fnUpper(s string) string {
	return strings.ToUpper(s)
}

func fnLower(s string) string {
	return strings.ToLower(s)
}

var (
	camelBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	upperRun      = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	nonAlphaNum   = regexp.MustCompile(`[^a-zA-Z0-9]+`)
)

func fnKebab(s string) string {
	if s == "" {
		return ""
	}
	// Insert hyphens at camelCase boundaries
	s = camelBoundary.ReplaceAllString(s, "${1}-${2}")
	s = upperRun.ReplaceAllString(s, "${1}-${2}")
	// Replace non-alphanumeric sequences with hyphen
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return strings.Map(unicode.ToLower, s)
}

func fnJoin(sep string, items []string) string {
	return strings.Join(items, sep)
}

func fnIndent(n int, s string) string {
	if s == "" {
		return ""
	}
	prefix := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
