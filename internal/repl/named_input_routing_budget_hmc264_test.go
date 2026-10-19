package repl

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

func TestHMC264ClassifierNamedZIPNavigationHasEncodedBudget(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for i := 0; i < 8; i++ {
		path := filepath.Join(dir, fmt.Sprintf("input-%d.zip", i))
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(f)
		count := 1
		if i < 4 {
			count = 64
		}
		for n := 0; n < count; n++ {
			name := fmt.Sprintf("exact-%d.data", n)
			if i < 4 {
				name = strings.Repeat("<", 180) + name
			}
			w, err := zw.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte("PERFILE2")); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	request := "分析这些采集材料： " + strings.Join(paths, " ")
	ctx := WithNamedInputRoutingContext(context.Background(), request, "", t.TempDir())
	adapter := &namedInputRoutingAdapter{}
	_, err := NewChitchatClassifier(adapter).(SingleShotTurnPolicyClassifier).ClassifyPolicySingleShot(ctx, request, "", false)
	if err != nil {
		t.Fatal(err)
	}
	var profileLine, prompt string
	for _, message := range adapter.messages {
		if message.Role == "user" {
			prompt = message.Content
		}
	}
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "[{\"path\"") || line == "[]" {
			profileLine = line
		}
	}
	var shown []hitraceconv.InputRoutingCapability
	if err := json.Unmarshal([]byte(profileLine), &shown); err != nil {
		t.Fatalf("navigation JSON: %v", err)
	}
	if len(profileLine) > 32<<10 || len(shown) == 0 || len(shown) >= 8 {
		t.Fatalf("encoded navigation budget absent: bytes=%d shown=%d", len(profileLine), len(shown))
	}
	if !strings.Contains(prompt, fmt.Sprintf("total=8 shown=%d omitted=%d", len(shown), 8-len(shown))) {
		t.Fatal("whole-record omission not disclosed")
	}
	for _, row := range shown {
		if row.ArchiveCandidates == nil || !row.ArchiveCandidates.MembersComplete || row.ArchiveCandidates.CandidateCount != len(row.ArchiveCandidates.Members) {
			t.Fatalf("budget truncated candidate set or changed counts: %+v", row)
		}
		for _, name := range row.ArchiveCandidates.Members {
			if name != "exact-0.data" {
				t.Fatalf("unexpected or truncated name: %q", name)
			}
		}
	}
	if !strings.Contains(prompt, "## current: "+request) {
		t.Fatal("navigation budget changed user request")
	}
}
