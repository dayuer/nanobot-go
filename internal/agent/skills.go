package agent

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SkillInfo describes a discovered skill.
type SkillInfo struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Source string `json:"source"` // "workspace" or "builtin"
}

// SkillsLoader discovers and loads agent skills from workspace and builtin dirs.
type SkillsLoader struct {
	Workspace      string
	WorkspaceSkills string
	BuiltinSkills  string
}

// NewSkillsLoader creates a SkillsLoader.
func NewSkillsLoader(workspace string, builtinSkillsDir string) *SkillsLoader {
	return &SkillsLoader{
		Workspace:       workspace,
		WorkspaceSkills: filepath.Join(workspace, "skills"),
		BuiltinSkills:   builtinSkillsDir,
	}
}

// ListSkills returns all available skills. Workspace skills override builtins.
func (s *SkillsLoader) ListSkills() []SkillInfo {
	var skills []SkillInfo
	seen := map[string]bool{}

	// Workspace skills (highest priority)
	if entries, err := os.ReadDir(s.WorkspaceSkills); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			skillFile := filepath.Join(s.WorkspaceSkills, e.Name(), "SKILL.md")
			if _, err := os.Stat(skillFile); err == nil {
				skills = append(skills, SkillInfo{Name: e.Name(), Path: skillFile, Source: "workspace"})
				seen[e.Name()] = true
			}
		}
	}

	// Builtin skills
	if s.BuiltinSkills != "" {
		if entries, err := os.ReadDir(s.BuiltinSkills); err == nil {
			for _, e := range entries {
				if !e.IsDir() || seen[e.Name()] {
					continue
				}
				skillFile := filepath.Join(s.BuiltinSkills, e.Name(), "SKILL.md")
				if _, err := os.Stat(skillFile); err == nil {
					skills = append(skills, SkillInfo{Name: e.Name(), Path: skillFile, Source: "builtin"})
				}
			}
		}
	}
	return skills
}

// LoadSkill loads a skill's content by name. Returns "" if not found.
func (s *SkillsLoader) LoadSkill(name string) string {
	// Workspace first
	wPath := filepath.Join(s.WorkspaceSkills, name, "SKILL.md")
	if data, err := os.ReadFile(wPath); err == nil {
		return string(data)
	}
	// Builtin
	if s.BuiltinSkills != "" {
		bPath := filepath.Join(s.BuiltinSkills, name, "SKILL.md")
		if data, err := os.ReadFile(bPath); err == nil {
			return string(data)
		}
	}
	return ""
}

// LoadSkillsForContext loads and formats specific skills for agent context.
func (s *SkillsLoader) LoadSkillsForContext(names []string) string {
	var parts []string
	for _, name := range names {
		content := s.LoadSkill(name)
		if content != "" {
			content = stripFrontmatter(content)
			parts = append(parts, "### Skill: "+name+"\n\n"+content)
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// BuildSkillsSummary returns an XML summary of all skills for progressive loading.
func (s *SkillsLoader) BuildSkillsSummary() string {
	skills := s.ListSkills()
	if len(skills) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "<skills>")
	for _, sk := range skills {
		desc := s.getSkillDescription(sk.Name)
		lines = append(lines, "  <skill available=\"true\">")
		lines = append(lines, "    <name>"+escapeXML(sk.Name)+"</name>")
		lines = append(lines, "    <description>"+escapeXML(desc)+"</description>")
		lines = append(lines, "    <location>"+sk.Path+"</location>")
		lines = append(lines, "  </skill>")
	}
	lines = append(lines, "</skills>")
	return strings.Join(lines, "\n")
}

// GetSkillMetadata parses YAML frontmatter from a skill.
func (s *SkillsLoader) GetSkillMetadata(name string) map[string]string {
	content := s.LoadSkill(name)
	if content == "" || !strings.HasPrefix(content, "---") {
		return nil
	}
	re := regexp.MustCompile(`(?s)^---\n(.*?)\n---`)
	match := re.FindStringSubmatch(content)
	if match == nil {
		return nil
	}
	meta := map[string]string{}
	for _, line := range strings.Split(match[1], "\n") {
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			val = strings.Trim(val, "\"'")
			meta[key] = val
		}
	}
	return meta
}

func (s *SkillsLoader) getSkillDescription(name string) string {
	meta := s.GetSkillMetadata(name)
	if meta != nil {
		if desc, ok := meta["description"]; ok && desc != "" {
			return desc
		}
	}
	return name
}

func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	re := regexp.MustCompile(`(?s)^---\n.*?\n---\n`)
	return strings.TrimSpace(re.ReplaceAllString(content, ""))
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
