package dispatch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// requiredSkill is the skill BuildPrompt renders into every sub-agent prompt.
// linkSkills provisions it into each worktree and verifies it lands so a
// missing skill fails at link time, not here.
const requiredSkill = "bender-implement-issue"

// BuildPrompt assembles the prompt sent to a sub-agent: the rendered
// bender-implement-issue SKILL.md from the worktree's .claude/skills/ dir,
// followed by the issue serialized as JSON.
func BuildPrompt(worktreePath string, issue schema.Issue) (string, error) {
	skillPath := filepath.Join(worktreePath, ".claude", "skills", requiredSkill, "SKILL.md")
	skill, err := os.ReadFile(skillPath)
	if err != nil {
		return "", fmt.Errorf("reading skill at %s: %w", skillPath, err)
	}

	body, err := json.MarshalIndent(issue, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling issue: %w", err)
	}

	return fmt.Sprintf("%s\n\n## Issue\n\n```json\n%s\n```\n", string(skill), string(body)), nil
}
