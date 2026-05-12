package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const writePrdSample = `{
  "name": "Test",
  "slug": "test",
  "status": "active",
  "created": "2026-03-26",
  "updated": "2026-03-26",
  "description": "A test",
  "why": "Because",
  "outcome": "Success"
}`

func TestWritePrd_ValidPrd(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))

	inputFile := filepath.Join(dir, "input.json")
	require.NoError(t, os.WriteFile(inputFile, []byte(writePrdSample), 0o644))

	cmd := NewWritePrdCmd()
	cmd.SetArgs([]string{"test", inputFile})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "wrote")
	_, err := os.Stat(filepath.Join(dir, ".plan-bender", "plans", "test", "prd.json"))
	assert.NoError(t, err)
}

func TestWritePrd_InvalidPrd(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	inputFile := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(inputFile, []byte(`{"slug": "x"}`), 0o644))

	cmd := NewWritePrdCmd()
	cmd.SetArgs([]string{"test", inputFile})
	cmd.SetOut(&strings.Builder{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

// TestWritePrd_RejectsUnknownFields guards against silent data loss at the
// CLI write boundary: a typo'd field used to be dropped on the way to disk,
// matching the planrepo loader's strict-decode contract on the read side.
func TestWritePrd_RejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))

	bad := strings.Replace(writePrdSample, `"outcome": "Success"`, `"outcom": "Success"`, 1)

	cmd := NewWritePrdCmd()
	cmd.SetArgs([]string{"test"})
	cmd.SetIn(strings.NewReader(bad))
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outcom")
}

// Uses os.Pipe (not strings.NewReader) so readInput's *os.File + non-char-device
// branch is exercised — the same path a shell heredoc hits in production.
func TestWritePrd_HeredocPipe(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))

	r, w, err := os.Pipe()
	require.NoError(t, err)
	go func() {
		defer w.Close()
		_, _ = w.WriteString(writePrdSample)
	}()
	t.Cleanup(func() { _ = r.Close() })

	info, err := r.Stat()
	require.NoError(t, err)
	require.Zero(t, info.Mode()&os.ModeCharDevice,
		"pipe read-end must not report as a character device — otherwise readInput would reject it")

	cmd := NewWritePrdCmd()
	cmd.SetArgs([]string{"test"})
	cmd.SetIn(r)
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "wrote")
	written, err := os.ReadFile(filepath.Join(dir, ".plan-bender", "plans", "test", "prd.json"))
	require.NoError(t, err)
	assert.Contains(t, string(written), `"name": "Test"`)
	assert.Contains(t, string(written), `"outcome": "Success"`)
}

func TestWritePrd_StdinPipe(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))

	cmd := NewWritePrdCmd()
	cmd.SetArgs([]string{"test"})
	cmd.SetIn(strings.NewReader(writePrdSample))
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "wrote")
	_, err := os.Stat(filepath.Join(dir, ".plan-bender", "plans", "test", "prd.json"))
	assert.NoError(t, err)
}
