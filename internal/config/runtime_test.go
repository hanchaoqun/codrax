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
	if err := os.WriteFile(path, []byte("log_stdout: false\n"), 0o644); err != nil {
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
	if s.LogDir != nil || s.LogLevel != nil || s.LogStdout != nil || s.MemoryDir != nil || s.Lang != nil {
		t.Errorf("empty file should leave all fields nil, got %+v", s)
	}
}
