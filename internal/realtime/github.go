package realtime

import "strings"

// GitHub is the GitHub realtime source. Like Slack, an empty repo list is valid
// and means a global search across every repo the user can access; a populated
// list filters the search by those repos.
type GitHub struct{}

const githubTitle = "GitHub (realtime)"

var githubTags = []string{"github", "repo", "code", "realtime"}

var githubKeywords = []string{
	"github", "repo", "repos", "repositório", "repositorio", "repositórios",
	"repositorios", "repository", "repositories", "pull request",
	"pull requests", "pr", "prs", "issue", "issues", "bug", "bugs", "commit",
	"commits", "branch", "branches", "merge", "merged", "review", "revisão",
	"revisao", "code review", "código", "codigo", "code", "release",
	"releases", "tag", "tags", "milestone", "label", "labels", "action",
	"actions", "ci", "workflow", "workflows", "build", "deploy", "contributor",
	"contribuidor", "fork", "star", "stars", "diff", "patch",
}

func (GitHub) ConfigPath() string { return configPath("github") }

func normalizeRepo(r map[string]string) Item {
	owner, name := r["owner"], r["name"]
	if owner == "" && name == "" {
		if fn := r["full_name"]; fn != "" {
			if i := strings.IndexByte(fn, '/'); i > 0 {
				owner, name = fn[:i], fn[i+1:]
			}
		}
	}
	return Item{"owner": owner, "name": name}
}

// SaveConfig normalizes and writes; an empty list is allowed (global search).
func (GitHub) SaveConfig(repos []map[string]string) ([]Item, error) {
	norm := []Item{}
	for _, r := range repos {
		n := normalizeRepo(r)
		if n["owner"] != "" && n["name"] != "" {
			norm = append(norm, n)
		}
	}
	if err := saveConfig("github", "repos", norm); err != nil {
		return nil, err
	}
	return norm, nil
}

func (GitHub) LoadConfig() ([]Item, bool) { return loadConfig("github", "repos") }

// Global indicates a search across every accessible repo (none connected).
func (GitHub) Global(repos []Item) bool {
	for _, r := range repos {
		if r["owner"] != "" && r["name"] != "" {
			return false
		}
	}
	return true
}

func (GitHub) Frontmatter(repos []Item) map[string]any {
	return map[string]any{
		"title": githubTitle, "kind": "realtime", "source": "github",
		"tags": githubTags, "repos": itemsAny(repos),
	}
}

func (g GitHub) Body(repos []Item) string {
	var scope string
	if g.Global(repos) {
		scope = "No specific repository connected: the search is **global** across every\n" +
			"repo the user can access, with no `repo:` filter.\n"
	} else {
		var b strings.Builder
		for _, r := range repos {
			b.WriteString("- " + formatRepo(r) + "\n")
		}
		scope = "Connected repositories — the search filters by them (GitHub search\n" +
			"OR-combines repeated `repo:owner/name` qualifiers in a single query):\n\n" +
			strings.TrimRight(b.String(), "\n") + "\n"
	}
	return "# " + githubTitle + "\n\n" +
		"This page is a **realtime source**: when `/vbrain-query-knowledge`\n" +
		"receives it as an FTS5 result, the agent does NOT return this body —\n" +
		"instead it calls the GitHub MCP search tools (issues, pull requests,\n" +
		"commits) with the user's query converted to GitHub search syntax.\n\n" +
		"## Scope\n\n" + scope + "\n" +
		"## Keywords (to match in FTS5)\n\n" +
		strings.Join(githubKeywords, ", ") + ".\n"
}

func (g GitHub) WriteWikiPage(repos []Item) (string, error) {
	return writePage("github", g.Frontmatter(repos), g.Body(repos))
}

func formatRepo(r Item) string {
	owner, name := r["owner"], r["name"]
	if owner == "" || name == "" {
		if name != "" {
			return "`" + name + "`"
		}
		return "`" + owner + "`"
	}
	return owner + "/" + name
}

// RepoFilter returns the GitHub search `repo:owner/name` qualifier ("" if the
// repo is incomplete).
func (GitHub) RepoFilter(r Item) string {
	if r["owner"] != "" && r["name"] != "" {
		return "repo:" + r["owner"] + "/" + r["name"]
	}
	return ""
}

// RepoFilterClause joins every repo's `repo:` qualifier with spaces. GitHub
// search OR-combines repeated `repo:` qualifiers, so a single query covers them
// all. Returns "" for global mode.
func (g GitHub) RepoFilterClause(repos []Item) string {
	var parts []string
	for _, r := range repos {
		if f := g.RepoFilter(r); f != "" {
			parts = append(parts, f)
		}
	}
	return strings.Join(parts, " ")
}
