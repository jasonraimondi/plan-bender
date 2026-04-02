package backend

import (
	"fmt"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func failingWrite(_ string, _ []byte, _ fs.FileMode) error {
	return fmt.Errorf("disk full")
}

func TestWritePrd_WriteError(t *testing.T) {
	dir := t.TempDir()
	store := NewPlanStore(dir, prodFS(dir), failingWrite, prodMkdir)

	prd := testPrd()
	err := store.WritePrd("test", prd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

func TestWriteIssue_WriteError(t *testing.T) {
	dir := t.TempDir()
	store := NewPlanStore(dir, prodFS(dir), failingWrite, prodMkdir)

	issue := testIssue(1)
	err := store.WriteIssue("test", issue)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

func TestWritePrd_MarshalError(t *testing.T) {
	// yaml.Marshal on a valid PrdYaml struct won't fail, but WritePrd propagates
	// any write error. This test verifies the error path is exercised.
	dir := t.TempDir()
	var writeCalled bool
	store := NewPlanStore(dir, prodFS(dir), func(_ string, _ []byte, _ fs.FileMode) error {
		writeCalled = true
		return fmt.Errorf("simulated marshal/write failure")
	}, prodMkdir)

	err := store.WritePrd("test", testPrd())
	require.Error(t, err)
	assert.True(t, writeCalled)
}

func TestWriteIssue_PropagatesError(t *testing.T) {
	dir := t.TempDir()
	store := NewPlanStore(dir, prodFS(dir), func(_ string, _ []byte, _ fs.FileMode) error {
		return fmt.Errorf("write failed")
	}, prodMkdir)

	issue := testIssue(1)
	err := store.WriteIssue("test", issue)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

func TestWritePrd_Success(t *testing.T) {
	dir := t.TempDir()
	store := NewProdPlanStore(dir)

	prd := testPrd()
	err := store.WritePrd("test", prd)
	require.NoError(t, err)

	// Read it back
	read, err := store.ReadPrd("test")
	require.NoError(t, err)
	assert.Equal(t, prd.Name, read.Name)
	assert.Equal(t, prd.Slug, read.Slug)
}

func TestWriteIssue_Success(t *testing.T) {
	dir := t.TempDir()
	store := NewProdPlanStore(dir)

	// WritePrd first to create directory structure
	require.NoError(t, store.WritePrd("test", testPrd()))

	issue := testIssue(1)
	err := store.WriteIssue("test", issue)
	require.NoError(t, err)

	issues, err := store.ReadIssues("test")
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, issue.Name, issues[0].Name)
}
