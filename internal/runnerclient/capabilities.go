package runnerclient

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/domain"
	"gopkg.in/yaml.v3"
)

const maxDiscoveredSkills = 128

type localSkill struct {
	Capability domain.AgentSkill
	Path       string
}

type skillRoot struct {
	Path   string
	Source string
}

type localResource struct {
	Capability domain.AgentResource
	Path       string
}

func discoverLocalCapabilities(agent config.RunnerAgent, configuredResources []config.RunnerResource, catalogRoots ...skillRoot) ([]string, []localSkill, []localResource) {
	roots := append([]skillRoot{}, catalogRoots...)
	for _, root := range agent.SkillRoots {
		roots = append(roots, skillRoot{Path: root, Source: "自定义目录"})
	}
	roots = uniqueExistingSkillRoots(roots)
	rootPaths := make([]string, 0, len(roots))
	for _, root := range roots {
		rootPaths = append(rootPaths, root.Path)
	}

	skills := make([]localSkill, 0)
	seenSkills := make(map[string]bool)
	for _, root := range roots {
		for _, skill := range scanSkillRoot(root.Path) {
			skill.Capability.Source = root.Source
			key := strings.ToLower(skill.Capability.Name)
			if seenSkills[key] {
				continue
			}
			seenSkills[key] = true
			skills = append(skills, skill)
			if len(skills) >= maxDiscoveredSkills {
				break
			}
		}
		if len(skills) >= maxDiscoveredSkills {
			break
		}
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Capability.Name < skills[j].Capability.Name })

	resources := []localResource{{Capability: domain.AgentResource{
		Name: "local-machine", Description: "Runner 当前系统用户可访问的本地文件、数据库、网络与应用", Access: "admin",
	}}}
	for _, resource := range configuredResources {
		if _, err := os.Stat(resource.Path); err != nil {
			continue
		}
		access := resource.Access
		if access == "" {
			access = "read_write"
		}
		resources = append(resources, localResource{Capability: domain.AgentResource{
			Name: resource.Name, Description: resource.Description, Access: access,
		}, Path: resource.Path})
	}
	return rootPaths, skills, resources
}

func uniqueExistingSkillRoots(roots []skillRoot) []skillRoot {
	result := make([]skillRoot, 0, len(roots))
	seen := make(map[string]bool)
	for _, root := range roots {
		clean := filepath.Clean(strings.TrimSpace(root.Path))
		if clean == "." || seen[clean] {
			continue
		}
		info, err := os.Stat(clean)
		if err != nil || !info.IsDir() {
			continue
		}
		seen[clean] = true
		root.Path = clean
		if strings.TrimSpace(root.Source) == "" {
			root.Source = "本机目录"
		}
		result = append(result, root)
	}
	return result
}

func scanSkillRoot(root string) []localSkill {
	result := make([]localSkill, 0)
	rootDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		depth := strings.Count(filepath.Clean(path), string(os.PathSeparator)) - rootDepth
		if entry.IsDir() && depth > 2 {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != "SKILL.md" {
			return nil
		}
		name := filepath.Base(filepath.Dir(path))
		result = append(result, localSkill{Capability: domain.AgentSkill{Name: name, Description: readSkillDescription(path)}, Path: path})
		if len(result) >= maxDiscoveredSkills {
			return filepath.SkipAll
		}
		return nil
	})
	return result
}

func readSkillDescription(path string) string {
	payload, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(payload) > 16_384 {
		payload = payload[:16_384]
	}
	text := string(payload)
	if strings.HasPrefix(strings.TrimSpace(text), "---") {
		lines := strings.Split(text, "\n")
		if len(lines) > 2 && strings.TrimSpace(lines[0]) == "---" {
			for index := 1; index < len(lines); index++ {
				if strings.TrimSpace(lines[index]) != "---" {
					continue
				}
				var frontmatter struct {
					Description string `yaml:"description"`
				}
				if yaml.Unmarshal([]byte(strings.Join(lines[1:index], "\n")), &frontmatter) == nil && strings.TrimSpace(frontmatter.Description) != "" {
					return truncateCapabilityDescription(frontmatter.Description)
				}
				break
			}
		}
	}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(strings.ToLower(line), "description:") {
			return truncateCapabilityDescription(strings.Trim(strings.TrimSpace(line[len("description:"):]), "\"'"))
		}
	}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "# ") {
			return truncateCapabilityDescription(strings.TrimSpace(strings.TrimPrefix(line, "# ")))
		}
	}
	return ""
}

func truncateCapabilityDescription(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 160 {
		return string(runes[:160]) + "…"
	}
	return string(runes)
}

func localCapabilityPrompt(item discoveredAgent, taskText string) string {
	if len(item.Skills) == 0 && len(item.Resources) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("本任务正在目标 Agent 所在电脑的 Runner 上执行。以下能力只在这台目标电脑本地可用；需要时请直接调用，不要假装调用。\n")
	if len(item.SkillRoots) > 0 {
		builder.WriteString("本机 Skill 根目录：\n")
		for _, root := range item.SkillRoots {
			builder.WriteString("- " + root + "\n")
		}
	}
	if len(item.Skills) > 0 {
		builder.WriteString("本机可用 Skill 名称：\n")
		names := make([]string, 0, len(item.Skills))
		for _, skill := range item.Skills {
			names = append(names, skill.Capability.Name)
		}
		builder.WriteString(strings.Join(names, "、"))
		builder.WriteString("\n")
		matchedSkills, _ := matchedCapabilityNames(taskText, item.Skills, nil)
		if len(matchedSkills) > 0 {
			matchedSet := make(map[string]bool, len(matchedSkills))
			for _, name := range matchedSkills {
				matchedSet[name] = true
			}
			builder.WriteString("本任务明确点名的 Skill 文件：\n")
			for _, skill := range item.Skills {
				if matchedSet[skill.Capability.Name] {
					builder.WriteString("- " + skill.Capability.Name + "：" + skill.Path + "\n")
				}
			}
		}
		builder.WriteString("如果任务适合某个 Skill，先读取对应 SKILL.md 并按其说明执行。\n")
	}
	if len(item.Resources) > 0 {
		builder.WriteString("本机资源：\n")
		for _, resource := range item.Resources {
			builder.WriteString(fmt.Sprintf("- %s [%s]", resource.Capability.Name, resource.Capability.Access))
			if resource.Path != "" {
				builder.WriteString("：" + resource.Path)
			}
			if resource.Capability.Description != "" {
				builder.WriteString("；" + resource.Capability.Description)
			}
			builder.WriteString("\n")
		}
	}
	builder.WriteString("最终结果中请如实说明实际调用的 Skill、文件、数据库或本地应用；没有调用时不要声称已调用。")
	return builder.String()
}

func matchedCapabilityNames(text string, skills []localSkill, resources []localResource) ([]string, []string) {
	lower := strings.ToLower(text)
	matchedSkills := make([]string, 0)
	for _, skill := range skills {
		if explicitlyMentionsSkill(lower, strings.ToLower(skill.Capability.Name)) {
			matchedSkills = append(matchedSkills, skill.Capability.Name)
		}
	}
	matchedResources := make([]string, 0)
	for _, resource := range resources {
		if explicitlyMentionsResource(lower, strings.ToLower(resource.Capability.Name)) {
			matchedResources = append(matchedResources, resource.Capability.Name)
		}
	}
	return matchedSkills, matchedResources
}

func currentTaskRequest(prompt string) string {
	const startMarker = "\n\n当前消息：\n"
	const endMarker = "\n\n可协作 Agent："
	start := strings.LastIndex(prompt, startMarker)
	if start < 0 {
		return prompt
	}
	current := prompt[start+len(startMarker):]
	if end := strings.Index(current, endMarker); end >= 0 {
		current = current[:end]
	}
	return strings.TrimSpace(current)
}

func explicitlyMentionsSkill(text, name string) bool {
	patterns := []string{
		name + " skill", name + " 技能", "skill " + name, "技能 " + name,
		"使用 " + name, "使用" + name, "用 " + name, "用" + name, "调用 " + name, "调用" + name,
		"/" + name + "/skill.md", "\\" + name + "\\skill.md",
	}
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func explicitlyMentionsResource(text, name string) bool {
	patterns := []string{
		name + " resource", name + " 本地资源", name + " 资源", "resource " + name, "资源 " + name,
		"使用 " + name, "使用" + name, "读取 " + name, "读取" + name,
	}
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}
