package operation

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CapabilitySnapshot is the compact, prompt-safe view of local command
// capabilities used by the command-operation planner. It is advisory only:
// deterministic approval/risk policy still lives in BuildCommandOperationPlan.
type CapabilitySnapshot struct {
	RepoRoot           string
	OS                 string
	Arch               string
	OSFamily           string
	OSVersion          string
	Shell              string
	GitRepoState       string
	InsideContainer    bool
	NetworkProbed      bool
	NetworkReachable   bool
	ProxySet           bool
	ProjectFiles       []CapabilityProjectFile
	PackageManagers    []CapabilityCommand
	Runtimes           []CapabilityCommand
	Commands           []CapabilityCommand
	OperationProviders []ProviderInfo
	Policy             CapabilityPolicy
}

type CapabilityCommand struct {
	Name    string
	Path    string
	Version string
	Source  string
}

type CapabilityProjectFile struct {
	Name string
	Path string
}

type CapabilityPolicy struct {
	AutoApprove        bool
	AutoLowRisk        bool
	TimeoutMS          int
	OutputPreviewBytes int
	UnknownProgram     string
	ShellPolicy        string
	NetworkPolicy      string
	InstallPolicy      string
	OverwritePolicy    string
	AllowedWriteRoots  []string
}

func BuildCapabilitySnapshot(facts *types.EnvFacts, repoRoot string, policy CommandPolicy) CapabilitySnapshot {
	return BuildCapabilitySnapshotWithProviders(facts, repoRoot, policy, nil)
}

func BuildCapabilitySnapshotWithProviders(facts *types.EnvFacts, repoRoot string, policy CommandPolicy, providers []ProviderInfo) CapabilitySnapshot {
	policy = normalizeCommandPolicy(policy)
	s := CapabilitySnapshot{
		RepoRoot:           strings.TrimSpace(repoRoot),
		OperationProviders: cleanProviderInfos(providers),
		Policy: CapabilityPolicy{
			AutoApprove:        policy.AutoApprove,
			AutoLowRisk:        policy.AutoLowRisk,
			TimeoutMS:          policy.TimeoutMS,
			OutputPreviewBytes: policy.OutputPreviewBytes,
			UnknownProgram:     policy.UnknownProgram,
			ShellPolicy:        policy.ShellPolicy,
			NetworkPolicy:      policy.NetworkPolicy,
			InstallPolicy:      policy.InstallPolicy,
			OverwritePolicy:    policy.OverwritePolicy,
			AllowedWriteRoots:  append([]string{}, policy.AllowedWriteRoots...),
		},
	}
	if s.RepoRoot != "" {
		if abs, err := filepath.Abs(s.RepoRoot); err == nil {
			s.RepoRoot = abs
		}
	}
	if facts != nil {
		s.OS = facts.OS
		s.Arch = facts.Arch
		s.OSFamily = facts.OSFamily
		s.OSVersion = facts.OSVersion
		s.Shell = facts.Shell
		s.GitRepoState = facts.GitRepoState
		s.InsideContainer = facts.InsideContainer
		s.NetworkProbed = !facts.Network.ProbedAt.IsZero()
		s.NetworkReachable = facts.Network.Reachable
		s.ProxySet = facts.Network.ProxySet
		s.ProjectFiles = capabilityProjectFiles(facts.ProjectFiles)
		s.PackageManagers = capabilityPackageManagers(facts.PkgManagers)
		s.Runtimes = capabilityRuntimes(facts)
	}
	known := append([]CapabilityCommand{}, s.PackageManagers...)
	known = append(known, s.Runtimes...)
	s.Commands = capabilityCommands(known)
	return s
}

func (s CapabilitySnapshot) RenderForPrompt() string {
	var b strings.Builder
	b.WriteString("## capability_snapshot\n")
	b.WriteString("Advisory local environment snapshot for choosing command plans. Policy still decides approval/risk.\n")
	if s.RepoRoot != "" {
		fmt.Fprintf(&b, "- repo_root: %s\n", s.RepoRoot)
	}
	fmt.Fprintf(&b, "- os: %s/%s", dashIfEmpty(s.OS), dashIfEmpty(s.Arch))
	if s.OSFamily != "" || s.OSVersion != "" {
		fmt.Fprintf(&b, " family=%s version=%s", dashIfEmpty(s.OSFamily), dashIfEmpty(s.OSVersion))
	}
	b.WriteString("\n")
	if s.Shell != "" || s.GitRepoState != "" {
		fmt.Fprintf(&b, "- shell: %s; git_repo_state: %s; inside_container: %t\n",
			dashIfEmpty(s.Shell), dashIfEmpty(s.GitRepoState), s.InsideContainer)
	}
	if s.NetworkProbed {
		fmt.Fprintf(&b, "- network: reachable=%t proxy=%t\n", s.NetworkReachable, s.ProxySet)
	} else {
		fmt.Fprintf(&b, "- network: not_probed proxy=%t\n", s.ProxySet)
	}
	fmt.Fprintf(&b, "- command_policy: auto_approve=%t auto_low_risk=%t timeout_ms=%d output_preview_bytes=%d unknown_program=%s shell_policy=%s\n",
		s.Policy.AutoApprove, s.Policy.AutoLowRisk, s.Policy.TimeoutMS, s.Policy.OutputPreviewBytes,
		dashIfEmpty(s.Policy.UnknownProgram), dashIfEmpty(s.Policy.ShellPolicy))
	fmt.Fprintf(&b, "- command_policy_controls: network=%s install=%s overwrite=%s allowed_write_roots=%s\n",
		dashIfEmpty(s.Policy.NetworkPolicy), dashIfEmpty(s.Policy.InstallPolicy),
		dashIfEmpty(s.Policy.OverwritePolicy), strings.Join(s.Policy.AllowedWriteRoots, ","))
	writeCommandList(&b, "package_managers", s.PackageManagers, 24)
	writeCommandList(&b, "runtimes", s.Runtimes, 16)
	writeCommandList(&b, "available_commands", s.Commands, 40)
	if len(s.ProjectFiles) > 0 {
		b.WriteString("- project_files:")
		for i, f := range s.ProjectFiles {
			if i >= 16 {
				fmt.Fprintf(&b, " ...(+%d)", len(s.ProjectFiles)-i)
				break
			}
			fmt.Fprintf(&b, " %s=%s", f.Name, f.Path)
		}
		b.WriteString("\n")
	}
	writeProviderList(&b, s.OperationProviders, 12)
	return strings.TrimSpace(b.String())
}

func cleanProviderInfos(values []ProviderInfo) []ProviderInfo {
	out := make([]ProviderInfo, 0, len(values))
	seen := map[string]bool{}
	for _, p := range values {
		name := strings.TrimSpace(p.Name)
		kind := strings.TrimSpace(p.Kind)
		if name == "" || kind == "" {
			continue
		}
		p.Name = name
		p.Kind = kind
		p.Surfaces = cleanStringList(p.Surfaces)
		p.SideEffects = cleanStringList(p.SideEffects)
		p.ToolName = strings.TrimSpace(p.ToolName)
		p.Description = strings.TrimSpace(p.Description)
		p.WhenToUse = cleanStringList(p.WhenToUse)
		p.WhenNotToUse = cleanStringList(p.WhenNotToUse)
		p.InputSchema = strings.TrimSpace(p.InputSchema)
		p.Examples = cleanStringList(p.Examples)
		p.Workflows = cleanWorkflowInfos(p.Workflows)
		p.Source = strings.TrimSpace(p.Source)
		key := p.Name + "\x00" + p.Kind + "\x00" + strings.TrimSpace(p.ToolName) + "\x00" + strings.Join(p.Surfaces, ",")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func cleanWorkflowInfos(values []WorkflowInfo) []WorkflowInfo {
	out := make([]WorkflowInfo, 0, len(values))
	seen := map[string]bool{}
	for _, w := range values {
		w.Name = strings.TrimSpace(w.Name)
		w.OperationKind = strings.TrimSpace(w.OperationKind)
		w.TargetSurface = strings.TrimSpace(w.TargetSurface)
		if w.Name == "" {
			continue
		}
		key := w.Name + "\x00" + w.OperationKind + "\x00" + w.TargetSurface
		if seen[key] {
			continue
		}
		seen[key] = true
		w.Summary = strings.TrimSpace(w.Summary)
		w.NextProviders = cleanStringList(w.NextProviders)
		w.ReturnProvider = strings.TrimSpace(w.ReturnProvider)
		w.RequiredInputs = cleanStringList(w.RequiredInputs)
		w.Examples = cleanStringList(w.Examples)
		w.Steps = cleanWorkflowStepInfos(w.Steps)
		out = append(out, w)
	}
	return out
}

func cleanWorkflowStepInfos(values []WorkflowStepInfo) []WorkflowStepInfo {
	out := make([]WorkflowStepInfo, 0, len(values))
	for _, s := range values {
		s.ID = strings.TrimSpace(s.ID)
		s.Provider = strings.TrimSpace(s.Provider)
		s.OperationKind = strings.TrimSpace(s.OperationKind)
		s.TargetSurface = strings.TrimSpace(s.TargetSurface)
		s.Description = strings.TrimSpace(s.Description)
		if s.ID == "" && s.Provider == "" && s.OperationKind == "" && s.TargetSurface == "" && s.Description == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func cleanStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func capabilityProjectFiles(files map[string]string) []CapabilityProjectFile {
	out := make([]CapabilityProjectFile, 0, len(files))
	for name, path := range files {
		name = strings.TrimSpace(name)
		path = strings.TrimSpace(path)
		if name == "" || path == "" {
			continue
		}
		out = append(out, CapabilityProjectFile{Name: name, Path: path})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func capabilityPackageManagers(values map[string]types.PkgManagerInfo) []CapabilityCommand {
	out := make([]CapabilityCommand, 0, len(values))
	for name, info := range values {
		name = strings.TrimSpace(name)
		if name == "" || strings.TrimSpace(info.Path) == "" {
			continue
		}
		out = append(out, CapabilityCommand{
			Name:    name,
			Path:    strings.TrimSpace(info.Path),
			Version: strings.TrimSpace(info.Version),
			Source:  "env_probe",
		})
	}
	sortCommands(out)
	return out
}

func capabilityRuntimes(f *types.EnvFacts) []CapabilityCommand {
	if f == nil {
		return nil
	}
	var out []CapabilityCommand
	for _, p := range f.Pythons {
		out = append(out, CapabilityCommand{Name: "python", Path: p.Path, Version: p.Version, Source: p.Origin})
	}
	for _, n := range f.Nodes {
		out = append(out, CapabilityCommand{Name: "node", Path: n.Path, Version: n.Version, Source: strings.Join(n.PkgManagers, ",")})
	}
	for _, r := range f.Rusts {
		out = append(out, CapabilityCommand{Name: "rustc", Path: r.RustcPath, Version: r.Version, Source: r.Origin})
		if r.CargoPath != "" {
			out = append(out, CapabilityCommand{Name: "cargo", Path: r.CargoPath, Version: r.Version, Source: r.Origin})
		}
	}
	for _, j := range f.Javas {
		out = append(out, CapabilityCommand{Name: "java", Path: j.JavacPath, Version: j.JavaVersion, Source: "env_probe"})
		if j.MavenPath != "" {
			out = append(out, CapabilityCommand{Name: "mvn", Path: j.MavenPath, Source: "env_probe"})
		}
		if j.GradlePath != "" {
			out = append(out, CapabilityCommand{Name: "gradle", Path: j.GradlePath, Source: "env_probe"})
		}
	}
	for _, r := range f.Rubies {
		out = append(out, CapabilityCommand{Name: "ruby", Path: r.RubyPath, Version: r.Version, Source: "env_probe"})
		if r.BundlerPath != "" {
			out = append(out, CapabilityCommand{Name: "bundle", Path: r.BundlerPath, Source: "env_probe"})
		}
	}
	out = compactCommands(out)
	return out
}

func capabilityCommands(known []CapabilityCommand) []CapabilityCommand {
	seen := map[string]bool{}
	for _, cmd := range known {
		if cmd.Name != "" {
			seen[cmd.Name] = true
		}
	}
	var out []CapabilityCommand
	for _, name := range operationCommonCommandCatalog() {
		if seen[name] {
			continue
		}
		path, err := exec.LookPath(name)
		if err != nil || strings.TrimSpace(path) == "" {
			continue
		}
		out = append(out, CapabilityCommand{Name: name, Path: path, Source: "look_path"})
	}
	sortCommands(out)
	return out
}

func operationCommonCommandCatalog() []string {
	return []string{
		"pwd", "ls", "find", "stat", "du", "df", "which", "cat", "head", "tail",
		"sed", "awk", "grep", "rg", "git", "go", "node", "npm", "python", "python3",
		"pip", "pip3", "mkdir", "mv", "cp", "rm", "curl", "wget", "tar", "zip", "unzip",
		"make", "cmake", "gcc", "clang", "java", "javac", "mvn", "gradle", "rustc", "cargo",
		"brew", "apt", "apt-get", "dnf", "yum", "pacman", "docker", "kubectl", "ssh", "scp",
		"rsync", "ffmpeg", "soffice", "libreoffice", "pandoc",
	}
}

func compactCommands(in []CapabilityCommand) []CapabilityCommand {
	seen := map[string]bool{}
	var out []CapabilityCommand
	for _, cmd := range in {
		cmd.Name = strings.TrimSpace(cmd.Name)
		cmd.Path = strings.TrimSpace(cmd.Path)
		if cmd.Name == "" || cmd.Path == "" || seen[cmd.Name+"|"+cmd.Path] {
			continue
		}
		seen[cmd.Name+"|"+cmd.Path] = true
		out = append(out, cmd)
	}
	sortCommands(out)
	return out
}

func sortCommands(values []CapabilityCommand) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Name == values[j].Name {
			return values[i].Path < values[j].Path
		}
		return values[i].Name < values[j].Name
	})
}

func writeCommandList(b *strings.Builder, label string, values []CapabilityCommand, limit int) {
	if len(values) == 0 {
		return
	}
	if limit <= 0 {
		limit = len(values)
	}
	fmt.Fprintf(b, "- %s:", label)
	for i, cmd := range values {
		if i >= limit {
			fmt.Fprintf(b, " ...(+%d)", len(values)-i)
			break
		}
		fmt.Fprintf(b, " %s=%s", cmd.Name, cmd.Path)
		if cmd.Version != "" {
			fmt.Fprintf(b, "(%s)", cmd.Version)
		}
	}
	b.WriteString("\n")
}

func writeProviderList(b *strings.Builder, values []ProviderInfo, limit int) {
	if len(values) == 0 {
		return
	}
	if limit <= 0 {
		limit = 12
	}
	b.WriteString("- operation_providers:\n")
	for i, p := range values {
		if i >= limit {
			fmt.Fprintf(b, "  ...(+%d provider descriptor(s))\n", len(values)-i)
			break
		}
		fmt.Fprintf(b, "  - %s kind=%s", p.Name, p.Kind)
		if len(p.Surfaces) > 0 {
			fmt.Fprintf(b, " surfaces=%s", strings.Join(p.Surfaces, ","))
		}
		if len(p.SideEffects) > 0 {
			fmt.Fprintf(b, " side_effects=%s", strings.Join(p.SideEffects, ","))
		}
		fmt.Fprintf(b, " gate=%t lazy=%t loaded=%t", p.RequiresGate, p.LazyStart, p.Loaded)
		if p.ToolName != "" {
			fmt.Fprintf(b, " tool=%s", p.ToolName)
		}
		if p.Source != "" {
			fmt.Fprintf(b, " source=%s", p.Source)
		}
		b.WriteString("\n")
		if p.Description != "" {
			fmt.Fprintf(b, "    description=%s\n", oneLineProviderText(p.Description, 220))
		}
		if len(p.WhenToUse) > 0 {
			fmt.Fprintf(b, "    when_to_use=%s\n", providerTextList(p.WhenToUse, 3, 140))
		}
		if len(p.WhenNotToUse) > 0 {
			fmt.Fprintf(b, "    when_not_to_use=%s\n", providerTextList(p.WhenNotToUse, 3, 140))
		}
		if p.InputSchema != "" {
			fmt.Fprintf(b, "    input_schema=%s\n", oneLineProviderText(p.InputSchema, 220))
		}
		if len(p.Examples) > 0 {
			fmt.Fprintf(b, "    examples=%s\n", providerTextList(p.Examples, 3, 160))
		}
		if len(p.Workflows) > 0 {
			writeProviderWorkflows(b, p.Workflows, 4)
		}
		if p.OutputContract.ArtifactRefs || p.OutputContract.PayloadRef || p.OutputContract.NextActions || p.OutputContract.ReturnAction || p.OutputContract.WorkflowState {
			fmt.Fprintf(b, "    output_contract=artifact_refs:%t payload_ref:%t next_actions:%t return_action:%t workflow_state:%t\n",
				p.OutputContract.ArtifactRefs, p.OutputContract.PayloadRef, p.OutputContract.NextActions, p.OutputContract.ReturnAction, p.OutputContract.WorkflowState)
		}
	}
}

func writeProviderWorkflows(b *strings.Builder, values []WorkflowInfo, limit int) {
	if limit <= 0 {
		limit = 4
	}
	for i, w := range values {
		if i >= limit {
			fmt.Fprintf(b, "    workflows=...(+%d)\n", len(values)-i)
			return
		}
		fmt.Fprintf(b, "    workflow[%s]", dashIfEmpty(w.Name))
		if w.Summary != "" {
			fmt.Fprintf(b, " summary=%s", oneLineProviderText(w.Summary, 180))
		}
		fmt.Fprintf(b, " entry=%t", w.Entry)
		if w.OperationKind != "" {
			fmt.Fprintf(b, " kind=%s", w.OperationKind)
		}
		if w.TargetSurface != "" {
			fmt.Fprintf(b, " target=%s", w.TargetSurface)
		}
		if len(w.NextProviders) > 0 {
			fmt.Fprintf(b, " next=%s", strings.Join(w.NextProviders, ","))
		}
		if w.ReturnProvider != "" {
			fmt.Fprintf(b, " return=%s", w.ReturnProvider)
		}
		if len(w.RequiredInputs) > 0 {
			fmt.Fprintf(b, " inputs=%s", strings.Join(w.RequiredInputs, ","))
		}
		b.WriteString("\n")
		if len(w.Steps) > 0 {
			fmt.Fprintf(b, "      steps=%s\n", workflowStepList(w.Steps, 4, 120))
		}
		if len(w.Examples) > 0 {
			fmt.Fprintf(b, "      workflow_examples=%s\n", providerTextList(w.Examples, 2, 140))
		}
	}
}

func providerTextList(values []string, limit, max int) string {
	if limit <= 0 {
		limit = len(values)
	}
	var out []string
	for _, value := range values {
		out = append(out, oneLineProviderText(value, max))
		if len(out) >= limit {
			break
		}
	}
	if len(values) > len(out) {
		out = append(out, fmt.Sprintf("...(+%d)", len(values)-len(out)))
	}
	return strings.Join(out, " | ")
}

func workflowStepList(values []WorkflowStepInfo, limit, max int) string {
	if limit <= 0 {
		limit = len(values)
	}
	var out []string
	for _, step := range values {
		var parts []string
		if step.ID != "" {
			parts = append(parts, step.ID)
		}
		if step.Provider != "" {
			parts = append(parts, "provider="+step.Provider)
		}
		if step.OperationKind != "" {
			parts = append(parts, "kind="+step.OperationKind)
		}
		if step.TargetSurface != "" {
			parts = append(parts, "target="+step.TargetSurface)
		}
		if step.Description != "" {
			parts = append(parts, oneLineProviderText(step.Description, max))
		}
		out = append(out, strings.Join(parts, " "))
		if len(out) >= limit {
			break
		}
	}
	if len(values) > len(out) {
		out = append(out, fmt.Sprintf("...(+%d)", len(values)-len(out)))
	}
	return strings.Join(out, " -> ")
}

func oneLineProviderText(s string, max int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if max <= 0 || len([]rune(s)) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "..."
}

func dashIfEmpty(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}
