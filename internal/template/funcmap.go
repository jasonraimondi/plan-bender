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
		"kebab":    fnKebab,
		"join":     fnJoin,
		"contains": fnContains,
	}
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

func fnContains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}
