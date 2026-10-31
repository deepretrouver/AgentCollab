package runnerclient

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/agentcatalog"
	"agentcollab.local/agentcollab/internal/config"
)

type discoveredAgent struct {
	Agent      config.RunnerAgent
	Available  bool
	Version    string
	Executable string
	SkillRoots []string
	Skills     []localSkill
	Resources  []localResource
}

func discoverRunnerAgents(cfg config.Runner) []discoveredAgent {
	catalog, err := loadDiscoveryCatalog(cfg.LocalDir)
	if err != nil {
		catalog, _ = agentcatalog.Default()
	}
	return discoverRunnerAgentsWithCatalog(cfg, catalog)
}

func discoverRunnerAgentsWithCatalog(cfg config.Runner, catalog agentcatalog.Catalog) []discoveredAgent {
	discovered := make([]discoveredAgent, 0, len(cfg.Agents)+len(catalog.Adapters))
	seenTypes := make(map[string]bool)
	catalogRoots := make(map[string][]skillRoot, len(catalog.Adapters))
	for _, adapter := range catalog.Adapters {
		catalogRoots[adapter.Type] = resolvedSkillRoots(adapter.SkillRoots)
	}
	for _, agent := range cfg.Agents {
		available := executableAvailable(agent)
		skillRoots, skills, resources := discoverLocalCapabilities(agent, cfg.Resources, catalogRoots[agent.Type]...)
		discovered = append(discovered, discoveredAgent{Agent: agent, Available: available, Version: probeVersion(agent), Executable: nativeExecutable(agent), SkillRoots: skillRoots, Skills: skills, Resources: resources})
		seenTypes[agent.Type] = true
	}
	for _, adapter := range catalog.Adapters {
		if seenTypes[adapter.Type] {
			continue
		}
		agent := resolveCatalogAdapter(adapter)
		available := agent.Command != "" && executableAvailable(agent)
		if !available && adapter.HideIfUnavailable {
			continue
		}
		skillRoots, skills, resources := discoverLocalCapabilities(agent, cfg.Resources, catalogRoots[adapter.Type]...)
		discovered = append(discovered, discoveredAgent{Agent: agent, Available: available, Version: probeVersion(agent), Executable: nativeExecutable(agent), SkillRoots: skillRoots, Skills: skills, Resources: resources})
	}
	return discovered
}

func loadDiscoveryCatalog(localDir string) (agentcatalog.Catalog, error) {
	catalog, err := agentcatalog.Default()
	if err != nil {
		return agentcatalog.Catalog{}, err
	}
	if strings.TrimSpace(localDir) == "" {
		return catalog, nil
	}
	if cached, loadErr := agentcatalog.Load(filepath.Join(localDir, "adapter-catalog.json")); loadErr == nil {
		catalog = cached
	}
	if override, loadErr := agentcatalog.Load(filepath.Join(localDir, "adapter-catalog.local.json")); loadErr == nil {
		return agentcatalog.Merge(catalog, override)
	}
	return catalog, nil
}

func resolveCatalogAdapter(adapter agentcatalog.Adapter) config.RunnerAgent {
	agent := config.RunnerAgent{
		ID: "scan-" + adapter.Type, DisplayName: adapter.DisplayName, Type: adapter.Type,
		Transport: adapter.Transport, CatalogTier: adapter.CatalogTier,
		Args: append([]string{}, adapter.Args...), ResumeArgs: append([]string{}, adapter.ResumeArgs...),
		VersionArgs: append([]string{}, adapter.VersionArgs...), OutputMode: adapter.OutputMode,
		PermissionMode: adapter.PermissionMode, ACPPermission: adapter.ACPPermission, ACPMode: adapter.ACPMode,
		PromptStdin: adapter.PromptStdin, TimeoutSeconds: adapter.TimeoutSeconds, NativeHistory: adapter.NativeHistory,
	}
	if agent.PermissionMode == "" {
		agent.PermissionMode = "admin"
	}
	if agent.TimeoutSeconds == 0 {
		agent.TimeoutSeconds = 900
	}
	for _, launcher := range adapter.Launchers {
		if !launcherSupportsOS(launcher.OS) {
			continue
		}
		command := findCatalogCommand(launcher)
		if command == "" {
			continue
		}
		entrypoint := expandCatalogPath(launcher.EntrypointPath)
		if entrypoint != "" {
			if info, err := os.Stat(entrypoint); err != nil || info.IsDir() {
				continue
			}
			agent.Args = append([]string{entrypoint}, agent.Args...)
			agent.ResumeArgs = append([]string{entrypoint}, agent.ResumeArgs...)
			agent.VersionArgs = append([]string{entrypoint}, agent.VersionArgs...)
		}
		agent.Command = command
		break
	}
	return agent
}

func launcherSupportsOS(supported []string) bool {
	if len(supported) == 0 {
		return true
	}
	for _, name := range supported {
		if name == runtime.GOOS {
			return true
		}
	}
	return false
}

func findCatalogCommand(launcher agentcatalog.Launcher) string {
	for _, name := range launcher.Commands {
		for _, candidate := range catalogCommandCandidates(name) {
			if candidate == name {
				if command, err := exec.LookPath(name); err == nil {
					if absolute, absErr := filepath.Abs(command); absErr == nil {
						return absolute
					}
				}
				continue
			}
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	for _, path := range launcher.CommandPaths {
		candidate := expandCatalogPath(path)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// catalogCommandCandidates supplements PATH for desktop-launched Clients.
// Windows GUI processes commonly do not inherit the user's npm PATH, while
// native CLI packages are normally installed in %APPDATA%\\npm or beside the
// desktop app. Keep this fallback data-only and bounded: it only probes known
// installation locations and never executes a shell.
func catalogCommandCandidates(name string) []string {
	home, _ := os.UserHomeDir()
	return catalogCommandCandidatesForOS(name, runtime.GOOS, home, os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"))
}

func catalogCommandCandidatesForOS(name, goos, home, appData, localAppData, programFiles, programFilesX86 string) []string {
	candidates := []string{name}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".local", "bin", name))
	}
	if goos != "windows" {
		return uniquePaths(candidates)
	}
	appData = strings.TrimSpace(appData)
	if appData == "" && home != "" {
		appData = filepath.Join(home, "AppData", "Roaming")
	}
	if appData != "" {
		for _, suffix := range []string{name + ".cmd", name + ".exe", name} {
			candidates = append(candidates, filepath.Join(appData, "npm", suffix))
		}
	}
	localAppData = strings.TrimSpace(localAppData)
	if localAppData == "" && home != "" {
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	programRoots := []string{localAppData, strings.TrimSpace(programFiles), strings.TrimSpace(programFilesX86)}
	for _, root := range programRoots {
		if root == "" {
			continue
		}
		for _, app := range []string{"WorkBuddy", "CodeBuddy"} {
			for _, suffix := range []string{"codebuddy.cmd", "codebuddy.exe", "codebuddy"} {
				candidates = append(candidates, filepath.Join(root, "Programs", app, "resources", "app.asar.unpacked", "cli", "bin", suffix))
				candidates = append(candidates, filepath.Join(root, app, "resources", "app.asar.unpacked", "cli", "bin", suffix))
			}
		}
	}
	return uniquePaths(candidates)
}

func uniquePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}

func resolvedSkillRoots(roots []agentcatalog.SkillRoot) []skillRoot {
	resolved := make([]skillRoot, 0, len(roots))
	for _, root := range roots {
		resolved = append(resolved, skillRoot{Path: expandCatalogPath(root.Path), Source: root.Source})
	}
	return resolved
}

func expandCatalogPath(path string) string {
	if path == "" {
		return ""
	}
	home, _ := os.UserHomeDir()
	return filepath.Clean(strings.ReplaceAll(path, "{home}", home))
}

func agentTransport(agent config.RunnerAgent) string {
	if agent.Transport == "" {
		return "native"
	}
	return agent.Transport
}

func agentCatalogTier(agent config.RunnerAgent) string {
	if agent.CatalogTier != "" {
		return agent.CatalogTier
	}
	if agentTransport(agent) == "acp" {
		return "custom"
	}
	return "native"
}

func executableAvailable(agent config.RunnerAgent) bool {
	if agent.Command == "" {
		return false
	}
	if info, err := os.Stat(agent.Command); err != nil || info.IsDir() {
		return false
	}
	if filepath.Base(agent.Command) == "node" && len(agent.Args) > 0 && filepath.IsAbs(agent.Args[0]) {
		if info, err := os.Stat(agent.Args[0]); err != nil || info.IsDir() {
			return false
		}
	}
	return true
}

func nativeExecutable(agent config.RunnerAgent) string {
	if filepath.Base(agent.Command) == "node" && len(agent.Args) > 0 && filepath.IsAbs(agent.Args[0]) {
		return agent.Args[0]
	}
	return agent.Command
}

func probeVersion(agent config.RunnerAgent) string {
	if !executableAvailable(agent) {
		return ""
	}
	args := agent.VersionArgs
	if len(args) == 0 {
		args = []string{"--version"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, agent.Command, args...).CombinedOutput()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(output))
	if newline := strings.IndexByte(line, '\n'); newline >= 0 {
		line = line[:newline]
	}
	if len(line) > 120 {
		line = line[:120]
	}
	return line
}
