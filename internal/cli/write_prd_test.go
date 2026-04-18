package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePrd_ValidPrd(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))

	prdYaml := `name: Test
slug: test
status: active
created: "2026-03-26"
updated: "2026-03-26"
description: A test
why: Because
outcome: Success
`
	inputFile := filepath.Join(dir, "input.yaml")
	require.NoError(t, os.WriteFile(inputFile, []byte(prdYaml), 0o644))

	cmd := NewWritePrdCmd()
	cmd.SetArgs([]string{"test", inputFile})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "wrote")
	_, err := os.Stat(filepath.Join(dir, ".plan-bender", "plans", "test", "prd.yaml"))
	assert.NoError(t, err)
}

func TestWritePrd_InvalidPrd(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	inputFile := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(inputFile, []byte("slug: x\n"), 0o644))

	cmd := NewWritePrdCmd()
	cmd.SetArgs([]string{"test", inputFile})
	cmd.SetOut(&strings.Builder{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

// Uses os.Pipe (not strings.NewReader) so readInput's *os.File + non-char-device
// branch is exercised — the same path a shell heredoc hits in production.
func TestWritePrd_HeredocPipe(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))

	heredocBody := `name: Test
slug: test
status: active
created: "2026-03-26"
updated: "2026-03-26"
description: A test
why: Because
outcome: Success
`
	r, w, err := os.Pipe()
	require.NoError(t, err)
	go func() {
		defer w.Close()
		_, _ = w.WriteString(heredocBody)
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
	written, err := os.ReadFile(filepath.Join(dir, ".plan-bender", "plans", "test", "prd.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(written), "name: Test")
	assert.Contains(t, string(written), "outcome: Success")
}

func TestWritePrd_StdinPipe(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))

	prdYaml := `name: Test
slug: test
status: active
created: "2026-03-26"
updated: "2026-03-26"
description: A test
why: Because
outcome: Success
`

	cmd := NewWritePrdCmd()
	cmd.SetArgs([]string{"test"})
	cmd.SetIn(strings.NewReader(prdYaml))
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "wrote")
	_, err := os.Stat(filepath.Join(dir, ".plan-bender", "plans", "test", "prd.yaml"))
	assert.NoError(t, err)
}
