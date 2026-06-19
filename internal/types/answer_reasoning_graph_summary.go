package types

import "strings"

func CloneAnswerReasoningGraphSummaryPtr(in *AnswerReasoningGraphSummary) *AnswerReasoningGraphSummary {
	if in == nil {
		return nil
	}
	out := NormalizeAnswerReasoningGraphSummary(*in)
	if answerReasoningGraphSummaryEmpty(out) {
		return nil
	}
	return &out
}

func NormalizeAnswerReasoningGraphSummary(in AnswerReasoningGraphSummary) AnswerReasoningGraphSummary {
	in.GraphID = strings.TrimSpace(in.GraphID)
	in.LastEventKind = strings.TrimSpace(in.LastEventKind)
	in.LastReasonCode = strings.TrimSpace(in.LastReasonCode)
	in.EventRefs = dedupTrimWriteWorkflowRunStrings(in.EventRefs)
	if in.EventCount < 0 {
		in.EventCount = 0
	}
	if in.ReadEventCount < 0 {
		in.ReadEventCount = 0
	}
	if in.ToolEventCount < 0 {
		in.ToolEventCount = 0
	}
	if in.RepairEventCount < 0 {
		in.RepairEventCount = 0
	}
	if in.LLMEventCount < 0 {
		in.LLMEventCount = 0
	}
	if in.NodeCount < 0 {
		in.NodeCount = 0
	}
	if in.EvidenceRefCount < 0 {
		in.EvidenceRefCount = 0
	}
	if in.AnswerBlockCount < 0 {
		in.AnswerBlockCount = 0
	}
	if in.CitationCount < 0 {
		in.CitationCount = 0
	}
	return in
}

func answerReasoningGraphSummaryEmpty(in AnswerReasoningGraphSummary) bool {
	return in.GraphID == "" &&
		in.EventCount == 0 &&
		in.LastEventKind == "" &&
		in.LastReasonCode == "" &&
		len(in.EventRefs) == 0 &&
		in.ReadEventCount == 0 &&
		in.ToolEventCount == 0 &&
		in.RepairEventCount == 0 &&
		in.LLMEventCount == 0 &&
		in.NodeCount == 0 &&
		in.EvidenceRefCount == 0 &&
		in.AnswerBlockCount == 0 &&
		in.CitationCount == 0
}
