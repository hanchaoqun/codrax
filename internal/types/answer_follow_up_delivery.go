package types

// answer_follow_up_delivery.go — HMC §377 D1: the typed receipt of the one
// optional follow-up the finalizer offers after an accepted answer.
//
// Batch 376 B offered the root-cause selection follow-up, refused its own
// request locally (707,552 B against the 700,000 B allocation) and the
// customer sidecar then said the model never selected — indistinguishable
// from a model that saw the offer and declined. The receipt records the
// system-side fact: the request was sent, or it was not and why.

// AnswerFollowUpNotSentCause is the closed reason a follow-up request was not
// sent.
type AnswerFollowUpNotSentCause string

const (
	// AnswerFollowUpNotSentRequestAllocation: the planned request still
	// exceeded the unchanged finalizer request allocation.
	AnswerFollowUpNotSentRequestAllocation AnswerFollowUpNotSentCause = "request_allocation"
	// AnswerFollowUpNotSentContextPressure: the planned request reached the
	// hard context-pressure ratio of the window.
	AnswerFollowUpNotSentContextPressure AnswerFollowUpNotSentCause = "context_pressure"
)

// AnswerFollowUpDelivery is one dispatch's follow-up decision. Byte values are
// local content-byte estimates of the planned request, not provider tokens.
type AnswerFollowUpDelivery struct {
	// RootCauseSelectionOffered: the follow-up offered the optional root-cause
	// selection for the separate JSON report.
	RootCauseSelectionOffered bool `json:"root_cause_selection_offered"`
	// Operations are the patch operations the follow-up published; empty means
	// the whole compatibility patch surface.
	Operations      []string                   `json:"operations,omitempty"`
	Sent            bool                       `json:"sent"`
	NotSentCause    AnswerFollowUpNotSentCause `json:"not_sent_cause,omitempty"`
	RequestBytes    int                        `json:"request_bytes"`
	AllocationBytes int                        `json:"allocation_bytes,omitempty"`
}

// RootCauseSelectionNotSent reports the one state the customer sidecar names:
// the follow-up offered the root-cause selection and its request was not sent.
func (d *AnswerFollowUpDelivery) RootCauseSelectionNotSent() bool {
	return d != nil && d.RootCauseSelectionOffered && !d.Sent
}

func cloneAnswerFollowUpDelivery(d *AnswerFollowUpDelivery) *AnswerFollowUpDelivery {
	if d == nil {
		return nil
	}
	c := *d
	c.Operations = append([]string(nil), d.Operations...)
	return &c
}

// SetAnswerFollowUpDelivery records this dispatch's follow-up decision.
func (m *MutableState) SetAnswerFollowUpDelivery(d *AnswerFollowUpDelivery) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.answerFollowUpDelivery = cloneAnswerFollowUpDelivery(d)
}

// AnswerFollowUpDelivery returns the latest follow-up decision, if any.
func (m *MutableState) AnswerFollowUpDelivery() *AnswerFollowUpDelivery {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneAnswerFollowUpDelivery(m.answerFollowUpDelivery)
}
