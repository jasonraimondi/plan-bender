package template

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRender_Variables(t *testing.T) {
	out, err := Render("test", "Hello {{.Name}}!", map[string]string{"Name": "World"})
	require.NoError(t, err)
	assert.Equal(t, "Hello World!", out)
}

func TestRender_Conditional(t *testing.T) {
	tmpl := `{{if .Show}}visible{{else}}hidden{{end}}`
	out, err := Render("test", tmpl, map[string]bool{"Show": true})
	require.NoError(t, err)
	assert.Equal(t, "visible", out)
}

func TestRender_Loop(t *testing.T) {
	tmpl := `{{range .Items}}- {{.}}
{{end}}`
	out, err := Render("test", tmpl, map[string][]string{"Items": {"a", "b", "c"}})
	require.NoError(t, err)
	assert.Equal(t, "- a\n- b\n- c\n", out)
}

func TestRender_ContainsPipe(t *testing.T) {
	tmpl := `{{if contains .Items "b"}}found{{else}}missing{{end}}`
	out, err := Render("test", tmpl, map[string]any{"Items": []string{"a", "b", "c"}})
	require.NoError(t, err)
	assert.Equal(t, "found", out)
}

func TestRender_KebabPipe(t *testing.T) {
	tmpl := `{{.Name | kebab}}`
	out, err := Render("test", tmpl, map[string]string{"Name": "HelloWorld"})
	require.NoError(t, err)
	assert.Equal(t, "hello-world", out)
}

func TestRender_JoinPipe(t *testing.T) {
	tmpl := `{{join ", " .Items}}`
	out, err := Render("test", tmpl, map[string][]string{"Items": {"a", "b"}})
	require.NoError(t, err)
	assert.Equal(t, "a, b", out)
}

func TestRender_ContainsNotFound(t *testing.T) {
	tmpl := `{{if contains .Items "z"}}found{{else}}missing{{end}}`
	out, err := Render("test", tmpl, map[string]any{"Items": []string{"a", "b"}})
	require.NoError(t, err)
	assert.Equal(t, "missing", out)
}

func TestRender_InvalidTemplate(t *testing.T) {
	_, err := Render("bad", "{{.Unclosed", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing template")
}

func TestLoadTemplates_Embedded(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)
	assert.Len(t, tmpls, 9)
	assert.Contains(t, tmpls, "bender-orchestrator.skill.tmpl")
}

func TestBuildContext_IncludesAgent(t *testing.T) {
	cfg := config.Defaults()
	ctx := BuildContext(cfg, config.ResolvedAgent{Name: "claude-code"})
	assert.Equal(t, "claude-code", ctx["agent"])
}

func TestBuildContext_DifferentAgents(t *testing.T) {
	cfg := config.Defaults()
	ctx1 := BuildContext(cfg, config.ResolvedAgent{Name: "claude-code"})
	ctx2 := BuildContext(cfg, config.ResolvedAgent{Name: "openclaw"})
	assert.Equal(t, "claude-code", ctx1["agent"])
	assert.Equal(t, "openclaw", ctx2["agent"])
}

func TestLoadTemplates_LocalOverride(t *testing.T) {
	dir := t.TempDir()
	overrideDir := filepath.Join(dir, ".plan-bender", "templates")
	require.NoError(t, os.MkdirAll(overrideDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(overrideDir, "custom.tmpl"),
		[]byte("custom content"),
		0o644,
	))

	tmpls, err := LoadTemplates(dir)
	require.NoError(t, err)
	assert.Equal(t, "custom content", tmpls["custom.tmpl"])
}
