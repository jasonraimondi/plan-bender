//go:build windows

package planrepo

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSnapshotUsesSlashSeparatedFSPaths(t *testing.T) {
	fsys := fstest.MapFS{
		"p/prd.yaml":          {Data: []byte(validPrd)},
		"p/issues/1-one.yaml": {Data: []byte(issueYAML(1, "one"))},
		"p/issues/2-two.yaml": {Data: []byte(issueYAML(2, "two"))},
	}

	snap, err := loadSnapshot(fsys, "p")
	require.NoError(t, err)
	assert.Equal(t, "p", snap.Slug)
	require.Len(t, snap.Issues, 2)
}

func TestFindIssueProjectUsesSlashSeparatedFSPaths(t *testing.T) {
	repo := New("", Adapters{FS: fstest.MapFS{
		"alpha/issues/7-one.yaml": {Data: []byte(issueYAML(7, "one"))},
		"beta/issues/8-two.yaml":  {Data: []byte(issueYAML(8, "two"))},
	}})

	slug, err := repo.FindIssueProject(7)
	require.NoError(t, err)
	assert.Equal(t, "alpha", slug)
}
