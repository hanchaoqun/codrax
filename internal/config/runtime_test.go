package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRuntimeSettings_Full(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "codrax.yaml")
	body := `
log_dir: custom_logs
log_level: debug
log_stdout: true
memory_dir: custom_memory
lang: en
thinking_truncate: true
repo: /tmp/project
branch: develop
pipeline_max_steps: 100
blob_max_inline_bytes: 65536
blob_preview_head_bytes: 49152
blob_preview_tail_bytes: 8192
readfile_small_limit_threshold: 50
pipeline_max_retries_per_stage: 5
pipeline_max_stage_visits: 6
analysis_warn_below_keywords: 6
analysis_reject_below_keywords: 3
analysis_generic_entity_blocklist:
  - thing
  - widget
analysis_reject_multiple_emit: true
analysis_max_prescan_rounds: 3
analysis_emit_only_correction_retries: 4
analysis_warn_below_keyword_hit_ratio: 0.5
analysis_warn_below_entity_hit_ratio: 0.75
analysis_evidence_profile: balanced
markdown_preview_server: on
markdown_preview_host: 127.0.0.1
markdown_preview_port: 49152
data_task_max_repair_rounds: 9
data_task_max_data_rounds: 18
write_workflow_engine: controller
providers_config: /etc/codrax/providers.yaml
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := LoadRuntimeSettings(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.LogDir == nil || *s.LogDir != "custom_logs" {
		t.Errorf("LogDir = %v", s.LogDir)
	}
	if s.LogLevel == nil || *s.LogLevel != "debug" {
		t.Errorf("LogLevel = %v", s.LogLevel)
	}
	if s.LogStdout == nil || *s.LogStdout != true {
		t.Errorf("LogStdout = %v", s.LogStdout)
	}
	if s.MemoryDir == nil || *s.MemoryDir != "custom_memory" {
		t.Errorf("MemoryDir = %v", s.MemoryDir)
	}
	if s.Lang == nil || *s.Lang != "en" {
		t.Errorf("Lang = %v", s.Lang)
	}
	if s.ThinkingTruncate == nil || *s.ThinkingTruncate != true {
		t.Errorf("ThinkingTruncate = %v", s.ThinkingTruncate)
	}
	if s.Repo == nil || *s.Repo != "/tmp/project" {
		t.Errorf("Repo = %v", s.Repo)
	}
	if s.Branch == nil || *s.Branch != "develop" {
		t.Errorf("Branch = %v", s.Branch)
	}
	if s.PipelineMaxSteps == nil || *s.PipelineMaxSteps != 100 {
		t.Errorf("PipelineMaxSteps = %v", s.PipelineMaxSteps)
	}
	if s.ProvidersConfig == nil || *s.ProvidersConfig != "/etc/codrax/providers.yaml" {
		t.Errorf("ProvidersConfig = %v", s.ProvidersConfig)
	}
	if s.WriteWorkflowEngine == nil || *s.WriteWorkflowEngine != "controller" {
		t.Errorf("WriteWorkflowEngine = %v", s.WriteWorkflowEngine)
	}
	// blob_*
	if s.BlobMaxInlineBytes == nil || *s.BlobMaxInlineBytes != 65536 {
		t.Errorf("BlobMaxInlineBytes = %v", s.BlobMaxInlineBytes)
	}
	if s.BlobPreviewHeadBytes == nil || *s.BlobPreviewHeadBytes != 49152 {
		t.Errorf("BlobPreviewHeadBytes = %v", s.BlobPreviewHeadBytes)
	}
	if s.BlobPreviewTailBytes == nil || *s.BlobPreviewTailBytes != 8192 {
		t.Errorf("BlobPreviewTailBytes = %v", s.BlobPreviewTailBytes)
	}
	// readfile_*
	if s.ReadFileSmallLimitThreshold == nil || *s.ReadFileSmallLimitThreshold != 50 {
		t.Errorf("ReadFileSmallLimitThreshold = %v", s.ReadFileSmallLimitThreshold)
	}
	// pipeline_*
	if s.PipelineMaxRetriesPerStage == nil || *s.PipelineMaxRetriesPerStage != 5 {
		t.Errorf("PipelineMaxRetriesPerStage = %v", s.PipelineMaxRetriesPerStage)
	}
	if s.PipelineMaxStageVisits == nil || *s.PipelineMaxStageVisits != 6 {
		t.Errorf("PipelineMaxStageVisits = %v", s.PipelineMaxStageVisits)
	}
	// analysis_*
	if s.AnalysisWarnBelowKeywords == nil || *s.AnalysisWarnBelowKeywords != 6 {
		t.Errorf("AnalysisWarnBelowKeywords = %v", s.AnalysisWarnBelowKeywords)
	}
	if s.AnalysisRejectBelowKeywords == nil || *s.AnalysisRejectBelowKeywords != 3 {
		t.Errorf("AnalysisRejectBelowKeywords = %v", s.AnalysisRejectBelowKeywords)
	}
	if len(s.AnalysisGenericEntityBlocklist) != 2 ||
		s.AnalysisGenericEntityBlocklist[0] != "thing" ||
		s.AnalysisGenericEntityBlocklist[1] != "widget" {
		t.Errorf("AnalysisGenericEntityBlocklist = %v", s.AnalysisGenericEntityBlocklist)
	}
	if s.AnalysisRejectMultipleEmit == nil || *s.AnalysisRejectMultipleEmit != true {
		t.Errorf("AnalysisRejectMultipleEmit = %v", s.AnalysisRejectMultipleEmit)
	}
	if s.AnalysisMaxPrescanRounds == nil || *s.AnalysisMaxPrescanRounds != 3 {
		t.Errorf("AnalysisMaxPrescanRounds = %v", s.AnalysisMaxPrescanRounds)
	}
	if s.AnalysisEmitOnlyCorrectionRetries == nil || *s.AnalysisEmitOnlyCorrectionRetries != 4 {
		t.Errorf("AnalysisEmitOnlyCorrectionRetries = %v", s.AnalysisEmitOnlyCorrectionRetries)
	}
	if s.AnalysisWarnBelowKeywordHitRatio == nil || *s.AnalysisWarnBelowKeywordHitRatio != 0.5 {
		t.Errorf("AnalysisWarnBelowKeywordHitRatio = %v", s.AnalysisWarnBelowKeywordHitRatio)
	}
	if s.AnalysisWarnBelowEntityHitRatio == nil || *s.AnalysisWarnBelowEntityHitRatio != 0.75 {
		t.Errorf("AnalysisWarnBelowEntityHitRatio = %v", s.AnalysisWarnBelowEntityHitRatio)
	}
	if s.AnalysisEvidenceProfile == nil || *s.AnalysisEvidenceProfile != "balanced" {
		t.Errorf("AnalysisEvidenceProfile = %v", s.AnalysisEvidenceProfile)
	}
	if s.MarkdownPreviewServer == nil || *s.MarkdownPreviewServer != "on" {
		t.Errorf("MarkdownPreviewServer = %v", s.MarkdownPreviewServer)
	}
	if s.MarkdownPreviewHost == nil || *s.MarkdownPreviewHost != "127.0.0.1" {
		t.Errorf("MarkdownPreviewHost = %v", s.MarkdownPreviewHost)
	}
	if s.MarkdownPreviewPort == nil || *s.MarkdownPreviewPort != 49152 {
		t.Errorf("MarkdownPreviewPort = %v", s.MarkdownPreviewPort)
	}
	if s.DataTaskMaxRepairRounds == nil || *s.DataTaskMaxRepairRounds != 9 {
		t.Errorf("DataTaskMaxRepairRounds = %v", s.DataTaskMaxRepairRounds)
	}
	if s.DataTaskMaxDataRounds == nil || *s.DataTaskMaxDataRounds != 18 {
		t.Errorf("DataTaskMaxDataRounds = %v", s.DataTaskMaxDataRounds)
	}
}

// TestLoadRuntimeSettings_MemoryRetrievalKnobs covers the magic
// numbers exposed in this commit: per-Kind retrieval policy + the
// two scoreIndex global tunables. Roundtrips a yaml fragment that
// overrides Pipeline.SessionPinCount + Chitchat.RecentBodyChars +
// the global EntityMinRunes / SessionTieBreakerBonus and asserts
// the parsed pointer-typed fields land in the right slots.
func TestLoadRuntimeSettings_MemoryRetrievalKnobs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "codrax.yaml")
	body := `
memory_entity_min_runes: 5
memory_session_tie_breaker_bonus: 4
memory_policy_pipeline:
  session_pin_count: 7
  compacted_match_cap: 12
memory_policy_chitchat:
  recent_body_chars: 2400
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := LoadRuntimeSettings(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.MemoryEntityMinRunes == nil || *s.MemoryEntityMinRunes != 5 {
		t.Errorf("MemoryEntityMinRunes = %v", s.MemoryEntityMinRunes)
	}
	if s.MemorySessionTieBreakerBonus == nil || *s.MemorySessionTieBreakerBonus != 4 {
		t.Errorf("MemorySessionTieBreakerBonus = %v", s.MemorySessionTieBreakerBonus)
	}
	if s.MemoryPolicyPipeline == nil {
		t.Fatal("MemoryPolicyPipeline missing")
	}
	if s.MemoryPolicyPipeline.SessionPinCount != 7 {
		t.Errorf("Pipeline.SessionPinCount = %d, want 7", s.MemoryPolicyPipeline.SessionPinCount)
	}
	if s.MemoryPolicyPipeline.CompactedMatchCap != 12 {
		t.Errorf("Pipeline.CompactedMatchCap = %d, want 12", s.MemoryPolicyPipeline.CompactedMatchCap)
	}
	if s.MemoryPolicyChitchat == nil || s.MemoryPolicyChitchat.RecentBodyChars != 2400 {
		t.Errorf("Chitchat.RecentBodyChars = %v", s.MemoryPolicyChitchat)
	}
	// Sub-policy fields not specified must stay zero so the memory
	// package's policyFor merge keeps the corresponding default.
	if s.MemoryPolicyPipeline.EntityScoreMul != 0 {
		t.Errorf("absent yaml field should parse as zero; EntityScoreMul = %d", s.MemoryPolicyPipeline.EntityScoreMul)
	}
	// Plan / Default were not configured — pointer must stay nil.
	if s.MemoryPolicyPlan != nil || s.MemoryPolicyDefault != nil {
		t.Error("unmentioned per-Kind sub-policies must parse as nil")
	}
}

func TestLoadRuntimeSettings_Partial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "codrax.yaml")
	// Only two keys — the rest must come back as nil pointers so the
	// caller knows to fall back to its own defaults.
	if err := os.WriteFile(path, []byte("log_level: warning\nlang: off\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := LoadRuntimeSettings(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.LogLevel == nil || *s.LogLevel != "warning" {
		t.Errorf("LogLevel = %v", s.LogLevel)
	}
	if s.Lang == nil || *s.Lang != "off" {
		t.Errorf("Lang = %v", s.Lang)
	}
	if s.LogDir != nil {
		t.Errorf("LogDir should be nil when absent, got %q", *s.LogDir)
	}
	if s.LogStdout != nil {
		t.Errorf("LogStdout should be nil when absent, got %v", *s.LogStdout)
	}
	if s.MemoryDir != nil {
		t.Errorf("MemoryDir should be nil when absent, got %q", *s.MemoryDir)
	}
}

func TestLoadRuntimeSettings_FalseIsNotAbsence(t *testing.T) {
	// Regression guard: if pointer fields were replaced with value
	// types, log_stdout: false would be indistinguishable from an
	// absent key. Keep this test to catch that regression.
	dir := t.TempDir()
	path := filepath.Join(dir, "codrax.yaml")
	if err := os.WriteFile(path, []byte("log_stdout: false\nthinking_truncate: false\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := LoadRuntimeSettings(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.LogStdout == nil {
		t.Fatalf("LogStdout should be non-nil when explicitly set to false")
	}
	if *s.LogStdout != false {
		t.Errorf("LogStdout = %v, want false", *s.LogStdout)
	}
	if s.ThinkingTruncate == nil {
		t.Fatalf("ThinkingTruncate should be non-nil when explicitly set to false")
	}
	if *s.ThinkingTruncate != false {
		t.Errorf("ThinkingTruncate = %v, want false", *s.ThinkingTruncate)
	}
}

func TestLoadRuntimeSettings_Missing(t *testing.T) {
	_, err := LoadRuntimeSettings(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !IsNotExist(err) {
		t.Errorf("expected IsNotExist, got %v", err)
	}
}

func TestLoadRuntimeSettings_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "codrax.yaml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := LoadRuntimeSettings(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// All fields must be nil — an empty file means "inherit defaults".
	if s.LogDir != nil || s.LogLevel != nil || s.LogStdout != nil ||
		s.MemoryDir != nil || s.Lang != nil ||
		s.Repo != nil || s.Branch != nil ||
		s.PipelineMaxSteps != nil ||
		s.ProvidersConfig != nil ||
		s.BlobMaxInlineBytes != nil || s.BlobPreviewHeadBytes != nil || s.BlobPreviewTailBytes != nil ||
		s.ReadFileSmallLimitThreshold != nil ||
		s.PipelineMaxRetriesPerStage != nil || s.PipelineMaxStageVisits != nil ||
		s.AnalysisWarnBelowKeywords != nil || s.AnalysisRejectBelowKeywords != nil ||
		s.AnalysisGenericEntityBlocklist != nil ||
		s.AnalysisRejectMultipleEmit != nil ||
		s.AnalysisMaxPrescanRounds != nil ||
		s.AnalysisEmitOnlyCorrectionRetries != nil ||
		s.AnalysisWarnBelowKeywordHitRatio != nil || s.AnalysisWarnBelowEntityHitRatio != nil ||
		s.MarkdownPreviewServer != nil || s.MarkdownPreviewHost != nil || s.MarkdownPreviewPort != nil {
		t.Errorf("empty file should leave all fields nil, got %+v", s)
	}
}
