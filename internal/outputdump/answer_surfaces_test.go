package outputdump

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnswerSurfaceArtifactBindsTranscriptAndNeverAffectsAnswer(t *testing.T) {
	for _, kind := range []string{"valid", "missing", "stale"} {
		t.Run(kind, func(t *testing.T) {
			a := Args{Dir: t.TempDir(), Answer: "full answer", Request: "question", PID: 9, Now: time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)}
			if kind != "missing" {
				a.AnswerSurfaces = &types.AnswerRenderedSurfaces{Answer: a.Answer, Primary: "model body", Principal: "model body\ncitations"}
			}
			if kind == "stale" {
				a.AnswerSurfaces.Answer = "older answer"
			}
			r := WriteResult(a)
			body, err := os.ReadFile(r.MarkdownPath)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(AnswerSurfacesPathForMarkdown(r.MarkdownPath))
			if err != nil {
				t.Fatal(err)
			}
			var s AnswerSurfaceArtifact
			if err := json.Unmarshal(raw, &s); err != nil {
				t.Fatal(err)
			}
			if string(body) != BuildBody(a) || s.MarkdownSHA256 != answerSurfaceDigest(string(body)) || s.AnswerSHA256 != answerSurfaceDigest(a.Answer) {
				t.Fatal("transcript binding changed")
			}
			if kind == "valid" {
				if s.Status != "available" || s.Primary != "model body" {
					t.Fatalf("lost audit: %+v", s)
				}
			} else if s.Status != "unavailable" || s.Primary != "" || s.Principal != "" {
				t.Fatalf("borrowed stale scope: %+v", s)
			}
			if !strings.Contains(string(body), "full answer") {
				t.Fatal("suppressed product answer")
			}
			PruneDir(a.Dir, 1)
			if _, err := os.Stat(AnswerSurfacesPathForMarkdown(r.MarkdownPath)); !os.IsNotExist(err) {
				t.Fatal("receipt not pruned with transcript")
			}
		})
	}
}

func TestAnswerSurfaceArtifactWriteFailureIsBestEffort(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	path := AnswerSurfacesPathForMarkdown(filepath.Join(dir, FileName(now, 1)))
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	r := WriteResult(Args{Dir: dir, Answer: "answer still ships", Now: now, PID: 1})
	if r.MarkdownPath == "" || r.HTMLPath == "" || r.RootCauseJSONError != nil {
		t.Fatalf("optional audit IO changed delivery: %+v", r)
	}
}
