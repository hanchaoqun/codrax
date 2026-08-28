package types

import (
	"strings"
	"testing"
)

func TestMergeEvidenceSummariesDeduplicatesOverlappingAcceptedBundles(t *testing.T) {
	got := MergeEvidenceSummaries(
		"字段使用 yaml 标签进行解析；字段使用 yaml 标签进行 YAML 解析；未设置时为 nil",
		"字段使用 yaml 标签进行 YAML 解析；未设置时为 nil；Decode 填充该字段",
	)
	for _, duplicate := range []string{
		"字段使用 yaml 标签进行 YAML 解析",
		"未设置时为 nil",
	} {
		if count := strings.Count(got, duplicate); count != 1 {
			t.Fatalf("summary atom %q count = %d, want 1: %q", duplicate, count, got)
		}
	}
	for _, want := range []string{
		"字段使用 yaml 标签进行解析",
		"字段使用 yaml 标签进行 YAML 解析",
		"未设置时为 nil",
		"Decode 填充该字段",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("distinct summary atom %q was lost: %q", want, got)
		}
	}
}

func TestMergeEvidenceSummariesKeepsDistinctAtomsAndFirstSeenOrder(t *testing.T) {
	got := MergeEvidenceSummaries("first claim；second claim", "third claim；first claim")
	if want := "first claim；second claim；third claim"; got != want {
		t.Fatalf("MergeEvidenceSummaries() = %q, want %q", got, want)
	}
}

func TestMergeEvidenceSummariesRetainsRicherContainingAtom(t *testing.T) {
	got := MergeEvidenceSummaries("Decode fills field", "Decode fills field with strict known-field checking")
	if want := "Decode fills field with strict known-field checking"; got != want {
		t.Fatalf("MergeEvidenceSummaries() = %q, want richer atom %q", got, want)
	}
}
