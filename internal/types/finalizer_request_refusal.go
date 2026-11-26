package types

// finalizer_request_refusal.go — the typed record of an answer request the
// byte guard refused before sending. One record serves every consumer: the
// scheduler's decision not to rebuild an identical request, the operator log,
// the reader notice and the root-cause side file's reason.

// FinalizerRequestRefusalKind says which request of the run's answer stage
// was refused.
type FinalizerRequestRefusalKind string

const (
	// FinalizerRefusalFirstRequest: the refused request was the first one of
	// its dispatch and no earlier answer dispatch of the run got far enough
	// to send one. Nothing of the answer stage reached the model.
	FinalizerRefusalFirstRequest FinalizerRequestRefusalKind = "first_request"
	// FinalizerRefusalLaterRequest: a repair or follow-up request. Either the
	// refused dispatch had already sent requests, or an earlier answer
	// dispatch of the run was not itself refused before sending.
	FinalizerRefusalLaterRequest FinalizerRequestRefusalKind = "repair"
)

// FinalizerRequestRefusal is the record of the run's last answer dispatch
// when the byte guard refused its request.
type FinalizerRequestRefusal struct {
	Kind FinalizerRequestRefusalKind
	// RequestBytes and AllocationBytes are the guard's two operands.
	RequestBytes    int
	AllocationBytes int
	// RequestsSent counts the requests the refused dispatch had already sent.
	RequestsSent int
	// InputDigest is the content digest of the typed inputs the refused
	// request was built from ("" when they could not be sampled).
	InputDigest string

	inputs TypedInputSample
}

// NothingSent reports whether the refusal left the answer stage without any
// request sent to the model. A nil record is not a refusal.
func (r *FinalizerRequestRefusal) NothingSent() bool {
	return r != nil && r.Kind == FinalizerRefusalFirstRequest && r.RequestsSent == 0
}

// InputsChangedSince compares the inputs the refused request was built from
// with a later sample. same is true only when both samples were read
// faithfully and are equal; changed names the carriers that differ.
func (r *FinalizerRequestRefusal) InputsChangedSince(now TypedInputSample) (changed []string, same bool) {
	if r == nil {
		return nil, false
	}
	if r.inputs.Same(now) {
		return nil, true
	}
	return r.inputs.Changed(now), false
}

type finalizeRequestLedger struct {
	// refusal is the last answer dispatch's refusal; nil when that dispatch
	// was not refused.
	refusal *FinalizerRequestRefusal
	// dispatchesNotRefused counts answer dispatches of this run that were not
	// refused before sending, so each may have sent a request.
	dispatchesNotRefused int
}

// RecordFinalizeRequestRefusal records that the answer dispatch which just
// returned was refused by the byte guard, and returns the record. inputs is
// the sample of the typed state taken after the dispatch returned.
func (m *MutableState) RecordFinalizeRequestRefusal(requestBytes, allocationBytes, requestsSent int, inputs TypedInputSample) *FinalizerRequestRefusal {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	record := &FinalizerRequestRefusal{
		Kind:            FinalizerRefusalLaterRequest,
		RequestBytes:    requestBytes,
		AllocationBytes: allocationBytes,
		RequestsSent:    requestsSent,
		InputDigest:     inputs.Digest(),
		inputs:          inputs,
	}
	if requestsSent == 0 && m.finalizeRequests.dispatchesNotRefused == 0 {
		record.Kind = FinalizerRefusalFirstRequest
	}
	if requestsSent > 0 {
		m.finalizeRequests.dispatchesNotRefused++
	}
	m.finalizeRequests.refusal = record
	copied := *record
	return &copied
}

// RecordFinalizeDispatchNotRefused records that the answer dispatch which
// just returned was not refused by the byte guard, whatever else it returned.
// The standing refusal, if any, no longer describes the run's last dispatch.
func (m *MutableState) RecordFinalizeDispatchNotRefused() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finalizeRequests.refusal = nil
	m.finalizeRequests.dispatchesNotRefused++
}

// FinalizerRequestRefusal returns the record of the run's last answer
// dispatch when the byte guard refused it, else nil.
func (m *MutableState) FinalizerRequestRefusal() *FinalizerRequestRefusal {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.finalizeRequests.refusal == nil {
		return nil
	}
	copied := *m.finalizeRequests.refusal
	return &copied
}
