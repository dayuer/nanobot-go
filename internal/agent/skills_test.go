package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSkillsDir(t *testing.T) (workspace, builtin string) {
	t.Helper()
	ws := t.TempDir()
	bi := t.TempDir()

	// Workspace skill
	os.MkdirAll(filepath.Join(ws, "skills", "coding", ""), 0o755)
	os.WriteFile(filepath.Join(ws, "skills", "coding", "SKILL.md"), []byte("---\ndescription: Code review\n---\n# Coding Skill\nDo reviews."), 0o644)

	// Builtin skill
	os.MkdirAll(filepath.Join(bi, "search", ""), 0o755)
	os.WriteFile(filepath.Join(bi, "search", "SKILL.md"), []byte("---\ndescription: Web search\n---\n# Search Skill"), 0o644)

	// Builtin skill with same name as workspace (should be overridden)
	os.MkdirAll(filepath.Join(bi, "coding", ""), 0o755)
	os.WriteFile(filepath.Join(bi, "coding", "SKILL.md"), []byte("---\ndescription: Builtin coding\n---\n# Old"), 0o644)

	return ws, bi
}

func TestSkillsLoader_ListSkills(t *testing.T) {
	ws, bi := setupSkillsDir(t)
	loader := NewSkillsLoader(ws, bi)
	skills := loader.ListSkills()
	assert.Len(t, skills, 2)

	// coding from workspace, search from builtin
	names := map[string]string{}
	for _, s := range skills {
		names[s.Name] = s.Source
	}
	assert.Equal(t, "workspace", names["coding"])
	assert.Equal(t, "builtin", names["search"])
}

func TestSkillsLoader_LoadSkill_Workspace(t *testing.T) {
	ws, bi := setupSkillsDir(t)
	loader := NewSkillsLoader(ws, bi)
	content := loader.LoadSkill("coding")
	assert.Contains(t, content, "# Coding Skill")
}

func TestSkillsLoader_LoadSkill_Builtin(t *testing.T) {
	ws, bi := setupSkillsDir(t)
	loader := NewSkillsLoader(ws, bi)
	content := loader.LoadSkill("search")
	assert.Contains(t, content, "# Search Skill")
}

func TestSkillsLoader_LoadSkill_NotFound(t *testing.T) {
	loader := NewSkillsLoader(t.TempDir(), "")
	assert.Equal(t, "", loader.LoadSkill("nonexistent"))
}

func TestSkillsLoader_WorkspaceOverridesBuiltin(t *testing.T) {
	ws, bi := setupSkillsDir(t)
	loader := NewSkillsLoader(ws, bi)
	content := loader.LoadSkill("coding")
	// Should get workspace version
	assert.Contains(t, content, "Code review")
	assert.NotContains(t, content, "Builtin coding")
}

func TestSkillsLoader_GetSkillMetadata(t *testing.T) {
	ws, bi := setupSkillsDir(t)
	loader := NewSkillsLoader(ws, bi)
	meta := loader.GetSkillMetadata("coding")
	require.NotNil(t, meta)
	assert.Equal(t, "Code review", meta["description"])
}

func TestSkillsLoader_GetSkillMetadata_NotFound(t *testing.T) {
	loader := NewSkillsLoader(t.TempDir(), "")
	assert.Nil(t, loader.GetSkillMetadata("nope"))
}

func TestSkillsLoader_LoadSkillsForContext(t *testing.T) {
	ws, bi := setupSkillsDir(t)
	loader := NewSkillsLoader(ws, bi)
	ctx := loader.LoadSkillsForContext([]string{"coding", "search"})
	assert.Contains(t, ctx, "### Skill: coding")
	assert.Contains(t, ctx, "### Skill: search")
	assert.Contains(t, ctx, "---") // separator
	// Frontmatter stripped
	assert.NotContains(t, ctx, "description:")
}

func TestSkillsLoader_BuildSkillsSummary(t *testing.T) {
	ws, bi := setupSkillsDir(t)
	loader := NewSkillsLoader(ws, bi)
	xml := loader.BuildSkillsSummary()
	assert.Contains(t, xml, "<skills>")
	assert.Contains(t, xml, "<name>coding</name>")
	assert.Contains(t, xml, "<description>Code review</description>")
	assert.Contains(t, xml, "</skills>")
}

func TestSkillsLoader_BuildSkillsSummary_Empty(t *testing.T) {
	loader := NewSkillsLoader(t.TempDir(), "")
	assert.Equal(t, "", loader.BuildSkillsSummary())
}

func TestStripFrontmatter(t *testing.T) {
	input := "---\nfoo: bar\n---\n# Content"
	assert.Equal(t, "# Content", stripFrontmatter(input))
}

func TestStripFrontmatter_None(t *testing.T) {
	assert.Equal(t, "No frontmatter", stripFrontmatter("No frontmatter"))
}

func TestEscapeXML(t *testing.T) {
	assert.Equal(t, "&lt;b&gt;text&amp;more&lt;/b&gt;", escapeXML("<b>text&more</b>"))
}
