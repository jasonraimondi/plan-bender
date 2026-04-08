package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKebab(t *testing.T) {
	tests := []struct{ in, want string }{
		{"HelloWorld", "hello-world"},
		{"helloWorld", "hello-world"},
		{"hello world", "hello-world"},
		{"hello_world", "hello-world"},
		{"HTTPServer", "http-server"},
		{"", ""},
		{"already-kebab", "already-kebab"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, fnKebab(tt.in), "kebab(%q)", tt.in)
	}
}

func TestJoin(t *testing.T) {
	tests := []struct {
		sep  string
		in   []string
		want string
	}{
		{", ", []string{"a", "b", "c"}, "a, b, c"},
		{"-", []string{}, ""},
		{" | ", []string{"one"}, "one"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, fnJoin(tt.sep, tt.in))
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name string
		list []string
		item string
		want bool
	}{
		{"present", []string{"a", "b", "c"}, "b", true},
		{"absent", []string{"a", "b", "c"}, "d", false},
		{"empty list", []string{}, "a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fnContains(tt.list, tt.item))
		})
	}
}
