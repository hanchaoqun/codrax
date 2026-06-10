package safety

import (
	"encoding/json"
	"encoding/xml"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	SignalLevelHigh     = "high"
	SignalLevelCritical = "critical"
)

// WriteContentSignal is a deterministic safety signal extracted from planned
// file content. It is intentionally about structured artifacts and protocol
// signatures, not user/model prose.
type WriteContentSignal struct {
	Code   string
	Detail string
	Level  string
}

// AnalyzeWriteContent extracts safety signals from the planned content for one
// file. Full-file content is preferred; patch payloads are treated as added
// structured fragments when possible.
func AnalyzeWriteContent(path, newContent, patch string) []WriteContentSignal {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	base := strings.ToLower(filepath.Base(clean))
	content := plannedContent(newContent, patch)
	added := diffAddedContent(patch)

	var out []WriteContentSignal
	if containsPEMPrivateKeyBoundary(content) {
		out = append(out, WriteContentSignal{
			Code:   "secret_material_in_change",
			Detail: "plan content contains private-key material",
			Level:  SignalLevelCritical,
		})
	}
	if packageJSONAddsLifecycleScript(base, content) || packageJSONAddsLifecycleScript(base, added) {
		out = append(out, WriteContentSignal{
			Code:   "dependency_lifecycle_script",
			Detail: "plan content adds or changes dependency lifecycle execution",
			Level:  SignalLevelHigh,
		})
	}
	if workflowPrivilegeEscalation(clean, base, content) || workflowPrivilegeEscalation(clean, base, added) {
		out = append(out, WriteContentSignal{
			Code:   "workflow_privilege_escalation",
			Detail: "plan content grants broad workflow privileges or sensitive token permissions",
			Level:  SignalLevelCritical,
		})
	}
	if permissionPolicyEscalation(clean, content) || permissionPolicyEscalation(clean, added) {
		out = append(out, WriteContentSignal{
			Code:   "permission_policy_escalation",
			Detail: "plan content requests sensitive platform permission or entitlement",
			Level:  SignalLevelHigh,
		})
	}
	if downloadExecutePayload(clean, base, content) || downloadExecutePayload(clean, base, added) {
		out = append(out, WriteContentSignal{
			Code:   "download_execute_payload",
			Detail: "plan content downloads remote data and executes it in a script or workflow surface",
			Level:  SignalLevelCritical,
		})
	}
	return dedupeSignals(out)
}

func plannedContent(newContent, patch string) string {
	newContent = strings.TrimSpace(newContent)
	if newContent != "" {
		return newContent
	}
	return diffAddedContent(patch)
}

func diffAddedContent(patch string) string {
	if strings.TrimSpace(patch) == "" {
		return ""
	}
	var lines []string
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+++") || !strings.HasPrefix(line, "+") {
			continue
		}
		lines = append(lines, strings.TrimPrefix(line, "+"))
	}
	return strings.Join(lines, "\n")
}

func containsPEMPrivateKeyBoundary(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		switch strings.TrimSpace(line) {
		case "-----BEGIN PRIVATE KEY-----",
			"-----BEGIN ENCRYPTED PRIVATE KEY-----",
			"-----BEGIN RSA PRIVATE KEY-----",
			"-----BEGIN DSA PRIVATE KEY-----",
			"-----BEGIN EC PRIVATE KEY-----",
			"-----BEGIN OPENSSH PRIVATE KEY-----":
			return true
		}
	}
	return false
}

func packageJSONAddsLifecycleScript(base, s string) bool {
	if base != "package.json" || strings.TrimSpace(s) == "" {
		return false
	}
	var doc struct {
		Scripts map[string]any `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		return false
	}
	for _, key := range []string{"preinstall", "install", "postinstall", "prepare"} {
		if _, ok := doc.Scripts[key]; ok {
			return true
		}
	}
	return false
}

func workflowPrivilegeEscalation(clean, base, s string) bool {
	if !isWorkflowOrAutomationPath(clean, base) || strings.TrimSpace(s) == "" {
		return false
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(s), &root); err != nil {
		return false
	}
	return yamlWorkflowEscalates(&root, "")
}

func yamlWorkflowEscalates(node *yaml.Node, parentKey string) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return yamlWorkflowEscalates(node.Content[0], parentKey)
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := scalarText(node.Content[i])
			value := node.Content[i+1]
			if key == "permissions" && yamlPermissionsEscalate(value) {
				return true
			}
			if (key == "contents" || key == "actions" || key == "id-token") && scalarText(value) == "write" {
				return true
			}
			if yamlWorkflowEscalates(value, key) {
				return true
			}
		}
		return false
	}
	if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			if yamlWorkflowEscalates(child, parentKey) {
				return true
			}
		}
		return false
	}
	if scalarText(node) == "pull_request_target" {
		return true
	}
	return false
}

func yamlPermissionsEscalate(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if scalarText(node) == "write-all" {
		return true
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := scalarText(node.Content[i])
			value := scalarText(node.Content[i+1])
			if (key == "contents" || key == "actions" || key == "id-token") && value == "write" {
				return true
			}
		}
	}
	return false
}

func permissionPolicyEscalation(clean, s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	lowerPath := strings.ToLower(clean)
	if strings.HasSuffix(lowerPath, "androidmanifest.xml") {
		return androidManifestEscalates(s)
	}
	return false
}

func androidManifestEscalates(s string) bool {
	decoder := xml.NewDecoder(strings.NewReader(s))
	for {
		tok, err := decoder.Token()
		if err != nil {
			return false
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "uses-permission" && start.Name.Local != "service" && start.Name.Local != "receiver" {
			continue
		}
		for _, attr := range start.Attr {
			if attr.Name.Local == "name" && sensitiveAndroidPermission(attr.Value) {
				return true
			}
			if attr.Name.Local == "permission" && sensitiveAndroidPermission(attr.Value) {
				return true
			}
		}
	}
}

func sensitiveAndroidPermission(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "ANDROID.PERMISSION.REQUEST_INSTALL_PACKAGES",
		"ANDROID.PERMISSION.SYSTEM_ALERT_WINDOW",
		"ANDROID.PERMISSION.MANAGE_EXTERNAL_STORAGE",
		"ANDROID.PERMISSION.WRITE_SECURE_SETTINGS",
		"ANDROID.PERMISSION.BIND_ACCESSIBILITY_SERVICE",
		"ANDROID.PERMISSION.BIND_DEVICE_ADMIN":
		return true
	default:
		return false
	}
}

func downloadExecutePayload(clean, base, s string) bool {
	if strings.TrimSpace(s) == "" || (!isWorkflowOrAutomationPath(clean, base) && !isExecutableScriptPath(clean, base)) {
		return false
	}
	for _, line := range strings.Split(s, "\n") {
		if shellLineDownloadsIntoShell(line) {
			return true
		}
	}
	return false
}

func shellLineDownloadsIntoShell(line string) bool {
	segments := splitShellPipeline(line)
	if len(segments) < 2 {
		return false
	}
	for i := 0; i+1 < len(segments); i++ {
		if shellSegmentProgram(segments[i]) == "curl" || shellSegmentProgram(segments[i]) == "wget" {
			next := shellSegmentProgram(segments[i+1])
			if next == "sh" || next == "bash" {
				return true
			}
		}
	}
	return false
}

func splitShellPipeline(line string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	escaped := false
	for _, r := range line {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			b.WriteRune(r)
			continue
		}
		if quote != 0 {
			b.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			b.WriteRune(r)
			continue
		}
		if r == '|' {
			out = append(out, b.String())
			b.Reset()
			continue
		}
		b.WriteRune(r)
	}
	out = append(out, b.String())
	return out
}

func shellSegmentProgram(segment string) string {
	tokens := shellFields(segment)
	if len(tokens) == 0 {
		return ""
	}
	for len(tokens) > 0 {
		switch tokens[0] {
		case "env", "command":
			tokens = tokens[1:]
			continue
		default:
			return strings.ToLower(filepath.Base(tokens[0]))
		}
	}
	return ""
}

func shellFields(s string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			flush()
			continue
		}
		b.WriteRune(r)
	}
	flush()
	return out
}

func scalarText(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(node.Value))
}

func isWorkflowOrAutomationPath(clean, base string) bool {
	lower := strings.ToLower(clean)
	if strings.HasPrefix(lower, ".github/workflows/") ||
		strings.HasPrefix(lower, ".circleci/") ||
		strings.HasPrefix(lower, ".buildkite/") ||
		strings.HasPrefix(lower, ".azure-pipelines/") ||
		strings.HasPrefix(lower, ".woodpecker/") ||
		strings.HasPrefix(lower, ".drone/") ||
		strings.HasPrefix(lower, "fastlane/") {
		return true
	}
	switch strings.ToLower(base) {
	case ".gitlab-ci.yml", ".gitlab-ci.yaml", "jenkinsfile", "azure-pipelines.yml",
		"azure-pipelines.yaml", "bitrise.yml", "bitrise.yaml", "codemagic.yaml",
		"codemagic.yml", ".travis.yml", "appveyor.yml", "buildkite.yml",
		"buildkite.yaml", ".drone.yml", ".drone.yaml", ".woodpecker.yml",
		".woodpecker.yaml":
		return true
	default:
		return false
	}
}

func isExecutableScriptPath(clean, base string) bool {
	lower := strings.ToLower(clean)
	if strings.HasPrefix(lower, "scripts/") || strings.Contains(lower, "/scripts/") ||
		strings.HasPrefix(lower, "bin/") || strings.Contains(lower, "/bin/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".sh", ".bash", ".zsh", ".fish", ".ps1", ".cmd", ".bat", ".command":
		return true
	default:
		return false
	}
}

func dedupeSignals(in []WriteContentSignal) []WriteContentSignal {
	seen := map[string]bool{}
	out := make([]WriteContentSignal, 0, len(in))
	for _, sig := range in {
		key := sig.Level + "\x00" + sig.Code + "\x00" + sig.Detail
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, sig)
	}
	return out
}
