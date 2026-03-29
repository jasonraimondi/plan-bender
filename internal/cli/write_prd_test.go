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
