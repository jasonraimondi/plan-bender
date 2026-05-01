package dispatch

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"gopkg.in/yaml.v3"
)

// BuildPrompt assembles the prompt sent to a sub-agent: the rendered
// bender-implement-issue SKILL.md from the worktree's .claude/skills/ dir,
// followed by the issue YAML serialized.
func BuildPrompt(worktreePath string, issue schema.IssueYaml) (string, error) {
	skillPath := filepath.Join(worktreePath, ".claude", "skills", "bender-implement-issue", "SKILL.md")
	skill, err := os.ReadFile(skillPath)
	if err != nil {
		return "", fmt.Errorf("reading skill at %s: %w", skillPath, err)
	}

	body, err := yaml.Marshal(issue)
	if err != nil {
		return "", fmt.Errorf("marshaling issue: %w", err)
	}

	return fmt.Sprintf("%s\n\n## Issue\n\n```yaml\n%s```\n", string(skill), string(body)), nil
}
