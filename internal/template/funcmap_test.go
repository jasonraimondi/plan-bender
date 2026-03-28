package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpper(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello", "HELLO"},
		{"", ""},
		{"Hello World", "HELLO WORLD"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, fnUpper(tt.in))
	}
}

func TestLower(t *testing.T) {
	tests := []struct{ in, want string }{
		{"HELLO", "hello"},
		{"", ""},
		{"Hello World", "hello world"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, fnLower(tt.in))
	}
}

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

func TestIndent(t *testing.T) {
	tests := []struct {
		n    int
		in   string
		want string
	}{
		{2, "line1\nline2", "  line1\n  line2"},
		{0, "no indent", "no indent"},
		{4, "", ""},
		{2, "single", "  single"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, fnIndent(tt.n, tt.in), "indent(%d, %q)", tt.n, tt.in)
	}
}
