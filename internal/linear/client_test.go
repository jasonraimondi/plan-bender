package linear

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	client := NewClient("lin_api_test_key")
	assert.NotNil(t, client)
	assert.NotNil(t, client.gql)
}

func TestIssueCreateInput_Fields(t *testing.T) {
	input := IssueCreateInput{
		Title:     "Test issue",
		TeamID:    "team-1",
		ProjectID: "proj-1",
		Priority:  2,
	}
	assert.Equal(t, "Test issue", input.Title)
	assert.Equal(t, "team-1", input.TeamID)
}

func TestIssueUpdateInput_Fields(t *testing.T) {
	input := IssueUpdateInput{
		Title:   "Updated",
		StateID: "state-1",
	}
	assert.Equal(t, "Updated", input.Title)
	assert.Equal(t, "state-1", input.StateID)
}
