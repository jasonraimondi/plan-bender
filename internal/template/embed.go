package template

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed all:embedded
var embeddedFS embed.FS

// Skill is one skill template: every file in the template directory keyed by its
// path relative to that directory (forward slashes), including SKILL.md.tmpl.
type Skill struct {
	Name  string
	Files map[string]string
}

// Main returns the skill body template (the SKILL.md.tmpl content).
func (s Skill) Main() string { return s.Files["SKILL.md.tmpl"] }

// LoadTemplates returns skill templates keyed by skill name. Bundled skills from
// embed.FS load first, then per-skill overrides from .plan-bender/templates/{name}/
// merge in at the file level (an override file replaces or adds an individual file).
func LoadTemplates(projectRoot string) (map[string]Skill, error) {
	skills := make(map[string]Skill)

	if err := fs.WalkDir(embeddedFS, "embedded", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, "embedded/")
		name, file, ok := strings.Cut(rel, "/")
		if !ok {
			return nil // a stray top-level file under embedded/ is not a skill
		}
		data, err := fs.ReadFile(embeddedFS, p)
		if err != nil {
			return err
		}
		addSkillFile(skills, name, file, string(data))
		return nil
	}); err != nil {
		return nil, err
	}

	if err := mergeOverrides(skills, filepath.Join(projectRoot, ".plan-bender", "templates")); err != nil {
		return nil, err
	}

	if err := validate(skills); err != nil {
		return nil, err
	}

	return skills, nil
}

func addSkillFile(skills map[string]Skill, name, file, content string) {
	s, ok := skills[name]
	if !ok {
		s = Skill{Name: name, Files: make(map[string]string)}
	}
	s.Files[file] = content
	skills[name] = s
}

// mergeOverrides walks .plan-bender/templates/{name}/ and upserts each file into
// the matching skill at the file level. A file directly under templates/ (no
// {name}/ dir) is ignored here; generate.go warns about that legacy flat layout.
func mergeOverrides(skills map[string]Skill, overrideRoot string) error {
	info, err := os.Stat(overrideRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(overrideRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(overrideRoot, p)
		if err != nil {
			return err
		}
		name, file, ok := strings.Cut(filepath.ToSlash(rel), "/")
		if !ok {
			return nil // flat file directly under templates/ is no longer a skill
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		addSkillFile(skills, name, file, string(data))
		return nil
	})
}

// validate enforces post-merge invariants: every skill template must contain a
// SKILL.md.tmpl body, and no two source files may produce the same output path
// (e.g. "X.tmpl" and "X", or "SKILL.md.tmpl" and a verbatim "SKILL.md").
func validate(skills map[string]Skill) error {
	var missing []string
	for name, s := range skills {
		if _, ok := s.Files["SKILL.md.tmpl"]; !ok {
			missing = append(missing, name)
		}
		seen := make(map[string]string, len(s.Files))
		for file := range s.Files {
			out := strings.TrimSuffix(file, ".tmpl")
			if prev, ok := seen[out]; ok {
				return fmt.Errorf("skill %q: files %q and %q both produce %q", name, prev, file, out)
			}
			seen[out] = file
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("skill templates missing SKILL.md.tmpl body: %s", strings.Join(missing, ", "))
	}
	return nil
}
