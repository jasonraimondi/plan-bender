package template

import (
	"bytes"
	"fmt"
	"text/template"
)

// Render executes a named template with the given content and context data.
func Render(name, content string, data any) (string, error) {
	tmpl, err := template.New(name).Funcs(FuncMap()).Parse(content)
	if err != nil {
		return "", fmt.Errorf("parsing template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template %s: %w", name, err)
	}

	return buf.String(), nil
}
