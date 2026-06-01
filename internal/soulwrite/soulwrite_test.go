package soulwrite_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/virtual360-io/vbrain/internal/page"
	"github.com/virtual360-io/vbrain/internal/soulwrite"
)

func TestCreatesSoulPagesUnderSoulDir(t *testing.T) {
	wiki := t.TempDir()
	res, err := soulwrite.SoulWrite([]soulwrite.PageInput{
		{Op: "create", Title: "Values freedom", BodyMarkdown: "I act for autonomy.", Tags: []string{"values"}},
	}, wiki)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 1 || len(res.Written) != 1 {
		t.Fatalf("res = %+v", res)
	}
	// The page must land inside _soul/, never at the wiki root.
	if !strings.HasPrefix(res.Written[0], "_soul/") {
		t.Fatalf("soul page must live under _soul/, got %q", res.Written[0])
	}
	full := filepath.Join(wiki, res.Written[0])
	parsed, err := page.Parse(full)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Frontmatter["kind"] != "soul" {
		t.Errorf("kind must be forced to soul, got %v", parsed.Frontmatter["kind"])
	}
}

func TestUpdateMergesTagsAndKeepsKind(t *testing.T) {
	wiki := t.TempDir()
	first, err := soulwrite.SoulWrite([]soulwrite.PageInput{
		{Op: "create", SlugHint: "freedom", Title: "Freedom", BodyMarkdown: "v1", Tags: []string{"a"}},
	}, wiki)
	if err != nil {
		t.Fatal(err)
	}
	slug := strings.TrimSuffix(strings.TrimPrefix(first.Written[0], "_soul/"), ".md")

	res, err := soulwrite.SoulWrite([]soulwrite.PageInput{
		{Op: "update", Slug: slug, Title: "Freedom", BodyMarkdown: "v2 refined", Tags: []string{"b"}},
	}, wiki)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 {
		t.Fatalf("res = %+v", res)
	}
	parsed, _ := page.Parse(filepath.Join(wiki, res.Updated[0]))
	body := parsed.Body
	if !strings.Contains(body, "v2 refined") {
		t.Errorf("body should be rewritten: %q", body)
	}
	tags := parsed.Frontmatter["tags"]
	got := toStringSlice(tags)
	if !contains(got, "a") || !contains(got, "b") {
		t.Errorf("tags should merge old+new, got %v", got)
	}
}

// A belief the user no longer holds must be prunable — the soul stays lean.
func TestDeleteRemovesSoulPage(t *testing.T) {
	wiki := t.TempDir()
	first, _ := soulwrite.SoulWrite([]soulwrite.PageInput{
		{Op: "create", SlugHint: "old-belief", Title: "Old belief", BodyMarkdown: "x"},
	}, wiki)
	slug := strings.TrimSuffix(strings.TrimPrefix(first.Written[0], "_soul/"), ".md")

	res, err := soulwrite.SoulWrite([]soulwrite.PageInput{{Op: "delete", Slug: slug}}, wiki)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Deleted) != 1 {
		t.Fatalf("res = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(wiki, "_soul", slug+".md")); !os.IsNotExist(err) {
		t.Errorf("deleted soul page should be gone")
	}
}

func toStringSlice(v any) []string {
	switch x := v.(type) {
	case []any:
		out := []string{}
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return x
	case string:
		return []string{x}
	}
	return nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
