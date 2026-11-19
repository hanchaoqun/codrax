package types

// answer_follow_up_delivery.go — HMC §377 D1: the typed receipt of the one
// optional follow-up the finalizer offers after an accepted answer.
//
// Batch 376 B offered the root-cause selection follow-up, refused its own
// request locally (707,552 B against the 700,000 B allocation) and the
// customer sidecar then said the model never selected — indistinguishable
// from a model that saw the offer and declined. The receipt records the
// system-side fact: the provider answered the request, or the request was not
// sent or its call failed, and why (HMC377 F6: a request counts as sent only
// once a provider response arrived).

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
	// AnswerFollowUpNotSentLoopStopped (HMC377 D1-R1/R4): the follow-up was
	// offered on the dispatch's last allowed iteration, or a force-stop ended
	// the loop, so no request decision was ever reached.
	AnswerFollowUpNotSentLoopStopped AnswerFollowUpNotSentCause = "loop_stopped"
	// AnswerFollowUpNotSentRequestFailed (HMC377 F6): the request was handed
	// to the provider but no response came back (provider error, stalled or
	// broken stream, request timeout, cancellation), so no answer of the model
	// to the offer was received.
	AnswerFollowUpNotSentRequestFailed AnswerFollowUpNotSentCause = "request_failed"
)

// AnswerFollowUpDelivery is one dispatch's follow-up outcome. Byte values are
// local content-byte estimates of the planned request, not provider tokens.
type AnswerFollowUpDelivery struct {
	// RootCauseSelectionOffered: the follow-up offered the optional root-cause
	// selection for the separate JSON report.
	RootCauseSelectionOffered bool `json:"root_cause_selection_offered"`
	// Operations are the patch operations the follow-up request actually
	// published (HMC377 D1-R5: what the schema projection applied, never the
	// declared union when the projection refused); empty means the whole
	// compatibility patch surface.
	Operations []string `json:"operations,omitempty"`
	// Sent: the provider answered the follow-up request, so the model's
	// response to the offer (a call or a decline) was received.
	Sent            bool                       `json:"sent"`
	NotSentCause    AnswerFollowUpNotSentCause `json:"not_sent_cause,omitempty"`
	RequestBytes    int                        `json:"request_bytes"`
	AllocationBytes int                        `json:"allocation_bytes,omitempty"`
}

// RootCauseSelectionNotSent reports the one state the customer sidecar names:
// the follow-up offered the root-cause selection and no provider response to
// it was received (not sent, or its request failed).
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
