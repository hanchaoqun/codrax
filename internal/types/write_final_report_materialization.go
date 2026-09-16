package types

const WriteFinalMaterializationSchemaVersion = 1

// WriteFinalMaterializationReceipt is a system-only projection of durable
// apply outcomes, never an LLM protocol or proof of successful verification.
// Owners is the complete historical apply/applied candidate roster, including
// test-only plans. It is NOT an instruction to overlay every candidate: a
// consumer must resolve RetainedPlanIDs as Git roots, select only candidate
// commits that are those roots or their Git ancestors, and validate that chain
// against its independently captured seed. Workflow Identity is not that seed.
// Unavailable receipts never publish a partial owner/root set.
type WriteFinalMaterializationReceipt struct {
	SchemaVersion   int                              `json:"schema_version"`
	Status          string                           `json:"status"`
	ReasonCode      string                           `json:"reason_code"`
	RunID           string                           `json:"run_id"`
	FinalPlanID     string                           `json:"final_plan_id"`
	RetainedPlanIDs []string                         `json:"retained_plan_ids"`
	Owners          []WriteFinalMaterializationOwner `json:"owners"`
}

type WriteFinalMaterializationOwner struct {
	PlanID    string `json:"plan_id"`
	CommitSHA string `json:"commit_sha"`
	// Paths is the checkpoint's committed/staged-path eligibility set, not
	// an exact Git delta. Consumers must verify actual delta is a subset;
	// a real checkpoint with an empty delta remains a valid owner.
	Paths []string `json:"paths"`
}

// Normalization only detaches storage. It must not manufacture availability,
// drop incomplete rows, or silently turn a historical candidate into a root.
func cloneWriteFinalMaterializationReceipt(in *WriteFinalMaterializationReceipt) *WriteFinalMaterializationReceipt {
	if in == nil {
		return nil
	}
	out := *in
	if in.RetainedPlanIDs != nil {
		out.RetainedPlanIDs = append([]string{}, in.RetainedPlanIDs...)
	}
	if in.Owners != nil {
		out.Owners = append([]WriteFinalMaterializationOwner{}, in.Owners...)
		for i := range out.Owners {
			if in.Owners[i].Paths != nil {
				out.Owners[i].Paths = append([]string{}, in.Owners[i].Paths...)
			}
		}
	}
	return &out
}
