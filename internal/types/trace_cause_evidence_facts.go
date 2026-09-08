package types

// Each contract snapshot owns its facts. Neither a caller nor a reader may
// change the stored scope or wakeup path through a shared pointer.
func cloneTraceCauseEvidenceFacts(in *TraceCauseEvidenceFacts) *TraceCauseEvidenceFacts {
	if in == nil {
		return nil
	}
	out := *in
	out.WakeupPath = append([]string(nil), in.WakeupPath...)
	if in.WindowScope != nil {
		scope := *in.WindowScope
		out.WindowScope = &scope
	}
	return &out
}
