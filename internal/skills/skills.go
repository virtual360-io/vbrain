// Package skills manages vbrain's OPTIONAL skills: those embedded under
// .claude/optional-skills (the catalog) that are NOT installed by default. The
// user opts in with `vbrain skill install <name>`; once installed they're kept
// current on every `vbrain update` (reinstalled from the new binary's embed).
//
// There is no separate state file: a skill is "installed" iff it is present in
// the base (<baseDir>/.claude/skills/<name>), which the base already versions.
// Operations mirror into the global home (~/.claude/skills) for the local
// session, exactly like the core skills.
package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/virtual360-io/vbrain/internal/scaffold"
	"gopkg.in/yaml.v3"
)

// Entry is one optional skill from the embed, flagged with whether it's
// installed in the base.
type Entry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Installed   bool   `json:"installed"`
}

// SyncResult reports an install/update sweep of the optional skills.
type SyncResult struct {
	Synced []string `json:"synced"` // (re)installed from the current embed
}

// RoutineSuggestion is the daily routine an optional skill recommends, declared
// in the skill's routine.yml. It carries no schedule — the time is chosen at
// install time (asked on a terminal, or by an agent via /vbrain-add-routine).
type RoutineSuggestion struct {
	Slug        string `json:"slug" yaml:"slug"`
	Description string `json:"description" yaml:"description"`
	Prompt      string `json:"prompt" yaml:"prompt"`
}

// SuggestedRoutine reads <name>/routine.yml from the embed, or nil if the skill
// declares no routine (or the file is incomplete).
func SuggestedRoutine(optFS fs.FS, name string) (*RoutineSuggestion, error) {
	data, err := fs.ReadFile(optFS, path.Join(name, "routine.yml"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var r RoutineSuggestion
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if strings.TrimSpace(r.Slug) == "" || strings.TrimSpace(r.Prompt) == "" {
		return nil, nil
	}
	return &r, nil
}

func installedPath(parent, name string) string {
	return filepath.Join(parent, ".claude", "skills", name)
}

func isInstalled(baseDir, name string) bool {
	fi, err := os.Stat(installedPath(baseDir, name))
	return err == nil && fi.IsDir()
}

// parents lists the install targets, base first; global is skipped when empty
// (UserHomeDir failed) or identical to base.
func parents(baseDir, globalHome string) []string {
	ps := []string{baseDir}
	if globalHome != "" && globalHome != baseDir {
		ps = append(ps, globalHome)
	}
	return ps
}

// CatalogNames lists the optional skill names available in the embed (sorted).
func CatalogNames(optFS fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(optFS, ".")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func inCatalog(optFS fs.FS, name string) (bool, error) {
	names, err := CatalogNames(optFS)
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if n == name {
			return true, nil
		}
	}
	return false, nil
}

func unknownSkillErr(optFS fs.FS, name string) error {
	names, _ := CatalogNames(optFS)
	return fmt.Errorf("unknown optional skill %q (available: %s)", name, strings.Join(names, ", "))
}

// Catalog returns the optional skills from the embed, each flagged installed if
// present in the base.
func Catalog(optFS fs.FS, baseDir string) ([]Entry, error) {
	names, err := CatalogNames(optFS)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(names))
	for _, name := range names {
		out = append(out, Entry{
			Name:        name,
			Description: skillDescription(optFS, name),
			Installed:   isInstalled(baseDir, name),
		})
	}
	return out, nil
}

// Install copies one optional skill from the embed into the base and the global
// home, after which it counts as installed. Validates name against the catalog.
func Install(optFS fs.FS, name, baseDir, globalHome string) error {
	ok, err := inCatalog(optFS, name)
	if err != nil {
		return err
	}
	if !ok {
		return unknownSkillErr(optFS, name)
	}
	for _, parent := range parents(baseDir, globalHome) {
		installed, _, err := scaffold.InstallNamedSkills(parent, optFS, []string{name})
		if err != nil {
			return err
		}
		if len(installed) == 0 {
			return fmt.Errorf("optional skill %q missing from the embed", name)
		}
	}
	return nil
}

// Remove deletes one optional skill from the base and the global home. It only
// accepts catalog names, so a typo (or a core skill name) can never delete a
// core skill. Returns whether it was installed in the base beforehand.
func Remove(optFS fs.FS, name, baseDir, globalHome string) (bool, error) {
	ok, err := inCatalog(optFS, name)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, unknownSkillErr(optFS, name)
	}
	wasInstalled := isInstalled(baseDir, name)
	for _, parent := range parents(baseDir, globalHome) {
		if err := os.RemoveAll(installedPath(parent, name)); err != nil {
			return wasInstalled, err
		}
	}
	return wasInstalled, nil
}

// Sync reinstalls — from the current embed — every optional skill already
// installed in the base, into the base and the global home. This is what keeps
// installed optionals up to date on `vbrain install`/`update`, and what
// materializes them in the global home of a freshly cloned base.
func Sync(optFS fs.FS, baseDir, globalHome string) (SyncResult, error) {
	names, err := CatalogNames(optFS)
	if err != nil {
		return SyncResult{}, err
	}
	var res SyncResult
	for _, name := range names {
		if !isInstalled(baseDir, name) {
			continue
		}
		for _, parent := range parents(baseDir, globalHome) {
			if _, _, err := scaffold.InstallNamedSkills(parent, optFS, []string{name}); err != nil {
				return res, err
			}
		}
		res.Synced = append(res.Synced, name)
	}
	return res, nil
}

func skillDescription(optFS fs.FS, name string) string {
	data, err := fs.ReadFile(optFS, path.Join(name, "SKILL.md"))
	if err != nil {
		return ""
	}
	return frontmatterDescription(string(data))
}

// frontmatterDescription pulls `description:` out of a SKILL.md YAML frontmatter
// (the leading --- … --- block); "" when there's none.
func frontmatterDescription(md string) string {
	s := strings.TrimLeft(md, "\uFEFF \t\r\n")
	if !strings.HasPrefix(s, "---") {
		return ""
	}
	rest := s[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return ""
	}
	var fm struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return ""
	}
	return strings.TrimSpace(fm.Description)
}
