package tracequery

import "strconv"

func ValidTransactionHandoffs(p TransactionHandoffsResult) bool {
	if p.SourcePath == "" || !isFiniteTraceNumber(p.Window.StartTs) || !isFiniteTraceNumber(p.Window.EndTs) || p.Window.EndTs < p.Window.StartTs || p.TargetPID < 0 {
		return false
	}
	if p.Status == "unavailable" {
		return p.Reason != "" && len(p.Handoffs) == 0 && p.TotalKeys == 0 && p.OmittedKeys == 0 && p.WindowSubmissionEvents == 0 && p.WindowConsumptionEvents == 0
	}
	if p.Status != "available" || p.TotalKeys != len(p.Handoffs)+p.OmittedKeys || p.OmittedKeys < 0 || len(p.Handoffs) > TransactionHandoffsLimit || p.WindowSubmissionEvents < 0 || p.WindowConsumptionEvents < 0 || p.MalformedProtocolEvents < 0 {
		return false
	}
	seen := map[transactionKey]bool{}
	type sourceLine struct {
		path string
		line int
	}
	windowSubmissions, windowConsumptions := map[sourceLine]bool{}, map[sourceLine]bool{}
	endpoints := map[sourceLine]TransactionEndpoint{}
	complete := p.OmittedKeys == 0
	for _, h := range p.Handoffs {
		seq, err := strconv.ParseUint(h.Sequence, 10, 64)
		if err != nil || strconv.FormatUint(seq, 10) != h.Sequence || h.TID <= 0 {
			return false
		}
		key := transactionKey{h.TID, h.Sequence}
		if seen[key] {
			return false
		}
		seen[key] = true
		complete = complete && h.OmittedSubmissions == 0 && h.OmittedConsumptions == 0
		if h.SubmissionCount != len(h.Submissions)+h.OmittedSubmissions || h.ConsumptionCount != len(h.Consumptions)+h.OmittedConsumptions || h.OmittedSubmissions < 0 || h.OmittedConsumptions < 0 || h.WindowSubmissions < 0 || h.WindowSubmissions > h.SubmissionCount || h.WindowConsumptions < 0 || h.WindowConsumptions > h.ConsumptionCount || h.WindowSubmissions+h.WindowConsumptions == 0 || len(h.Submissions) > TransactionEndpointExamplesLimit || len(h.Consumptions) > TransactionEndpointExamplesLimit {
			return false
		}
		for i, es := range [][]TransactionEndpoint{h.Submissions, h.Consumptions} {
			lines := map[int]bool{}
			windowCount := 0
			for _, e := range es {
				if e.SourcePath == "" || e.SourceLine <= 0 || e.Line <= 0 || lines[e.Line] || !isFiniteTraceNumber(e.Ts) || e.Name == "" || e.InWindow != (e.Ts >= p.Window.StartTs && (e.Ts < p.Window.EndTs || e.Ts == p.Window.EndTs && p.Window.EndInclusive)) {
					return false
				}
				lines[e.Line] = true
				identity := sourceLine{e.SourcePath, e.SourceLine}
				if old, found := endpoints[identity]; found && old != e {
					return false
				}
				endpoints[identity] = e
				if e.InWindow {
					windowCount++
					if i == 0 {
						windowSubmissions[identity] = true
					} else {
						windowConsumptions[identity] = true
					}
				}
				role, keys, bad := transactionProtocol(Event{Type: EventTraceMark, SpanAction: "B", SpanName: e.Name})
				if bad || i == 0 && role != "submission" || i == 1 && role != "consumption" {
					return false
				}
				found := false
				for _, k := range keys {
					found = found || k == key
				}
				if !found {
					return false
				}
			}
			wanted, omitted := h.WindowSubmissions, h.OmittedSubmissions
			if i == 1 {
				wanted, omitted = h.WindowConsumptions, h.OmittedConsumptions
			}
			if windowCount > wanted || windowCount+omitted < wanted {
				return false
			}
		}
		switch h.Status {
		case "observed_unique_protocol_match":
			if h.SubmissionCount != 1 || h.ConsumptionCount != 1 || len(h.Submissions) != 1 || len(h.Consumptions) != 1 {
				return false
			}
			s, c := h.Submissions[0], h.Consumptions[0]
			if s.SourcePath != c.SourcePath || s.TID != h.TID || s.TGID <= 0 || c.TID <= 0 || c.TGID <= 0 || c.Ts < s.Ts || c.Ts == s.Ts && c.Line <= s.Line {
				return false
			}
		case "ambiguous":
			if h.SubmissionCount <= 1 && h.ConsumptionCount <= 1 {
				return false
			}
		case "missing_submission":
			if h.SubmissionCount != 0 || h.ConsumptionCount != 1 {
				return false
			}
		case "missing_consumption":
			if h.ConsumptionCount != 0 || h.SubmissionCount != 1 {
				return false
			}
		case "identity_unverified", "order_unverified":
			if h.SubmissionCount != 1 || h.ConsumptionCount != 1 || h.Reason == "" {
				return false
			}
		default:
			return false
		}
	}
	if len(windowSubmissions) > p.WindowSubmissionEvents || len(windowConsumptions) > p.WindowConsumptionEvents {
		return false
	}
	if complete && (len(windowSubmissions) != p.WindowSubmissionEvents || len(windowConsumptions) != p.WindowConsumptionEvents) {
		return false
	}
	return true
}
