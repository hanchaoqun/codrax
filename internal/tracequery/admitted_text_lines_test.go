package tracequery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

func TestStreamAdmittedTraceTextLinesNormalParityAndEarlyStop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "markers.systrace")
	body := "first\r\nsecond\nthird"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var observed []AdmittedTraceTextLine
	result, err := StreamAdmittedTraceTextLines(context.Background(), path, func(line AdmittedTraceTextLine) bool {
		observed = append(observed, line)
		return line.Number < 2
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ScannedLines != 2 || !result.Stopped {
		t.Fatalf("early-stop accounting drifted: %+v", result)
	}
	if len(observed) != 2 || observed[0].Number != 1 || observed[0].Text != "first" || observed[1].Number != 2 || observed[1].Text != "second" {
		t.Fatalf("physical line parity drifted: %+v", observed)
	}
}

func TestStreamAdmittedTraceTextLinesRejectsLateBinaryBeforeCallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "late-binary.systrace")
	body := strings.Repeat("safe text\n", 9000) + "late\x00binary\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	callbacks := 0
	_, err := StreamAdmittedTraceTextLines(context.Background(), path, func(AdmittedTraceTextLine) bool {
		callbacks++
		return true
	})
	var admission *TraceInputAdmissionError
	if !errors.As(err, &admission) || admission.Code != TraceInputAdmissionCodeConversionRequired {
		t.Fatalf("late binary did not retain typed fail-closed admission: %T %v", err, err)
	}
	if callbacks != 0 {
		t.Fatalf("late binary reached raw line consumer before full-file admission: callbacks=%d", callbacks)
	}
}

func TestStreamAdmittedTraceTextLinesRejectsOversizedPhysicalLineBeforeCallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.systrace")
	body := strings.Repeat("x", attachment.TracePhysicalLineMaxBytes+1) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	callbacks := 0
	_, err := StreamAdmittedTraceTextLines(context.Background(), path, func(AdmittedTraceTextLine) bool {
		callbacks++
		return true
	})
	var admission *TraceInputAdmissionError
	if !errors.As(err, &admission) || admission.Code != TraceInputAdmissionCodeLineTooLong {
		t.Fatalf("oversized physical line did not retain typed admission: %T %v", err, err)
	}
	if callbacks != 0 {
		t.Fatalf("oversized physical line reached raw line consumer: callbacks=%d", callbacks)
	}
}

func TestStreamAdmittedTraceTextLinesRejectsPostAdmissionPathReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replace.systrace")
	detached := filepath.Join(dir, "detached.systrace")
	body := "first\nsecond\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	replaced := false
	result, err := StreamAdmittedTraceTextLines(context.Background(), path, func(line AdmittedTraceTextLine) bool {
		if !replaced {
			replaced = true
			if renameErr := os.Rename(path, detached); renameErr != nil {
				t.Fatalf("detach admitted generation: %v", renameErr)
			}
			if writeErr := os.WriteFile(path, []byte(body), 0o600); writeErr != nil {
				t.Fatalf("install path replacement: %v", writeErr)
			}
		}
		return false
	})
	if result.ScannedLines != 1 || !result.Stopped {
		t.Fatalf("replacement seam did not run after one admitted line: %+v", result)
	}
	if err == nil || (!strings.Contains(err.Error(), "source path was replaced") && !strings.Contains(err.Error(), "source identity changed")) {
		t.Fatalf("post-admission path replacement escaped final binding: %v", err)
	}
}
