package attachment

import (
	"strings"
	"testing"
)

func TestValidateTextAcceptsRuntimeText(t *testing.T) {
	data := []byte("2026-06-08 10:00:00 WARN service started\n\tframe: foo.bar\n")
	if err := ValidateText(KindLog, "run.log", data, false); err != nil {
		t.Fatalf("text log should be accepted: %v", err)
	}
}

func TestValidateTextRejectsNULBinary(t *testing.T) {
	err := ValidateText(KindTrace, "capture.htrace", []byte{'H', 'T', 0, 1, 2, 3}, false)
	if err == nil {
		t.Fatal("binary trace should be rejected")
	}
	issue, ok := err.(TextIssue)
	if !ok {
		t.Fatalf("err type = %T, want TextIssue", err)
	}
	if issue.Reason != "contains NUL bytes" {
		t.Fatalf("reason=%q", issue.Reason)
	}
	msg := issue.Message("zh", SurfaceREPL)
	for _, want := range []string{"/htrace convert", "codrax trace convert", "capture.htrace"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q: %s", want, msg)
		}
	}
}

func TestValidateTextRejectsInvalidUTF8(t *testing.T) {
	err := ValidateText(KindLog, "bad.log", []byte{0xff, 0xfe, 'x'}, false)
	if err == nil {
		t.Fatal("invalid UTF-8 should be rejected")
	}
	if !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("error should mention UTF-8: %v", err)
	}
}

func TestValidateTextAllowsTruncatedUTF8Tail(t *testing.T) {
	data := []byte("正常日志行 ")
	data = append(data, []byte("中")[:2]...)
	if err := ValidateText(KindLog, "truncated.log", data, true); err != nil {
		t.Fatalf("truncated UTF-8 tail at cap boundary should be accepted: %v", err)
	}
}

func TestValidateTextRejectsEmptyUntruncated(t *testing.T) {
	err := ValidateText(KindLog, "empty.log", nil, false)
	if err == nil {
		t.Fatal("empty attachment should be rejected")
	}
	if !strings.Contains(err.Error(), "no readable content") {
		t.Fatalf("empty message should be transparent: %v", err)
	}
}
