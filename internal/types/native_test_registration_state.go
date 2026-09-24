package types

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// NativeTestRegistrationAuthorization is controller-owned dispatch authority,
// never model input or a persisted permission to execute tests.
type NativeTestRegistrationAuthorization struct {
	ID, RunID, BatchID, RepositoryRoot string
	Delivery                           VerificationDeliverySnapshot
	Contracts                          []WriteBehaviorContract
	TargetPaths                        []string
	contextDigest                      string
}

type nativeTestRegistrationExecution struct{ digest, root, contextDigest string }

func cloneNativeRegistrationAuthorization(in *NativeTestRegistrationAuthorization) *NativeTestRegistrationAuthorization {
	if in == nil {
		return nil
	}
	out := *in
	out.Delivery = CloneVerificationDeliverySnapshot(in.Delivery)
	body, _ := json.Marshal(in.Contracts)
	_ = json.Unmarshal(body, &out.Contracts)
	out.TargetPaths = append([]string(nil), in.TargetPaths...)
	return &out
}

func nativeRegistrationProofBatch(run *WriteWorkflowRun) *WriteWorkflowBatch {
	if run == nil || run.RunID == "" || run.ActiveBatchID == "" {
		return nil
	}
	for i := range run.Batches {
		b := &run.Batches[i]
		if b.ID == run.ActiveBatchID && (b.Purpose == "verification_proof_followup" || b.Purpose == "impact_and_verification_proof_followup") {
			return b
		}
	}
	return nil
}

// Called with m.mu held. Typed context changes invalidate the grant even when
// a replacement contract happens to reuse the same ID.
func (m *MutableState) nativeRegistrationContextDigestLocked() string {
	run := m.writeWorkflowRun
	b := nativeRegistrationProofBatch(run)
	if b == nil {
		return ""
	}
	return nativeRegistrationHash(struct {
		RunID, BatchID, Purpose string
		IR                      *WriteAnalysisIR
		Tombstones              []WriteBehaviorContractTombstone
		Generation              WriteBehaviorContractResolution
	}{run.RunID, b.ID, b.Purpose, m.writeAnalysisIR, m.behaviorContractTombstoneLedger.Rows(), m.projectBehaviorContractGenerationLocked(nil, nil)})
}

func (m *MutableState) AuthorizeNativeTestRegistration(root string, delivery VerificationDeliverySnapshot, contracts []WriteBehaviorContract, targets []string) error {
	if m == nil {
		return fmt.Errorf("missing registration context")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nativeTestRegistrationAuthorization = nil
	digest := m.nativeRegistrationContextDigestLocked()
	if digest == "" || !delivery.Valid() || !filepath.IsAbs(root) || filepath.Clean(root) != root || len(contracts) == 0 || len(targets) == 0 {
		return fmt.Errorf("registration requires one current source delivery and active proof-followup contracts")
	}
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	g := &NativeTestRegistrationAuthorization{ID: hex.EncodeToString(token[:]), RunID: m.writeWorkflowRun.RunID, BatchID: m.writeWorkflowRun.ActiveBatchID, RepositoryRoot: root, Delivery: delivery, Contracts: contracts, TargetPaths: targets, contextDigest: digest}
	m.nativeTestRegistrationAuthorization = cloneNativeRegistrationAuthorization(g)
	return nil
}

func (m *MutableState) NativeTestRegistrationAuthorization() *NativeTestRegistrationAuthorization {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	g := m.nativeTestRegistrationAuthorization
	if g == nil || g.contextDigest != m.nativeRegistrationContextDigestLocked() {
		return nil
	}
	return cloneNativeRegistrationAuthorization(g)
}

func (m *MutableState) RevokeNativeTestRegistrationAuthorization() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nativeTestRegistrationAuthorization = nil
}

func (m *MutableState) InstallNativeTestRegistration(plan *ChangePlan, authorizationID string, tests []NativeTestFileVersion) error {
	g := m.NativeTestRegistrationAuthorization()
	if g == nil || g.ID != authorizationID || plan == nil || plan.ID == "" {
		return fmt.Errorf("registration authorization expired")
	}
	if len(plan.Changes) != 0 || len(plan.Slices) != 0 || len(plan.VerificationProbes) != 0 || len(plan.ProjectTestObservations) == 0 ||
		len(plan.SupersededBehaviorContracts) != 0 || len(plan.SupersededBehaviorContractIDs) != 0 ||
		nativeRegistrationHash(plan.BehaviorContracts) != nativeRegistrationHash(g.Contracts) {
		return fmt.Errorf("registration may only bind existing tests to the unchanged active contracts")
	}
	// Read checks acquire their own lock; the generation is checked again in
	// the publication critical section so a reset cannot lend an old read.
	generation := m.BeginDispatchRepositoryFileRead()
	for _, test := range tests {
		if !m.CompleteDispatchDeliveredRepositoryFileReadVersion(g.RepositoryRoot, test.Path, test.SHA256) {
			return fmt.Errorf("existing test version was not fully delivered to this planning dispatch: %s", test.Path)
		}
	}
	candidate := *plan
	m.mu.RLock()
	analysisJSON, analysisErr := json.Marshal(m.writeAnalysisIR)
	m.mu.RUnlock()
	if analysisErr != nil {
		return analysisErr
	}
	candidate.WriteAnalysisIR = nil
	if err := json.Unmarshal(analysisJSON, &candidate.WriteAnalysisIR); err != nil {
		return err
	}
	candidate.PersistenceKind = PlanPersistenceNativeTestRegistration
	candidate.TargetPaths = append([]string(nil), g.TargetPaths...)
	candidate.Status = PlanStatusNoChangeRequired
	candidate.CumulativeVerificationScope = &CumulativeVerificationScope{SourcePlanIDs: []string{g.Delivery.SourcePlanID}, TargetPaths: append([]string(nil), g.TargetPaths...), AppliedSources: []VerificationDeliverySnapshot{CloneVerificationDeliverySnapshot(g.Delivery)}}
	candidate.NativeTestRegistration = &NativeTestRegistration{Version: 1, PlanID: plan.ID, AuthorizationID: g.ID, RunID: g.RunID, BatchID: g.BatchID, RepositoryRoot: g.RepositoryRoot, Delivery: CloneVerificationDeliverySnapshot(g.Delivery), Tests: append([]NativeTestFileVersion(nil), tests...)}
	sealNativeTestRegistration(&candidate)
	digest := NativeTestRegistrationDigest(&candidate)
	if digest == "" {
		return fmt.Errorf("invalid read-only registration shape")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current := m.nativeTestRegistrationAuthorization
	if current == nil || current.ID != g.ID || current.contextDigest != m.nativeRegistrationContextDigestLocked() || generation != m.dispatchRepositoryFileReadGeneration {
		return fmt.Errorf("registration context changed before publication")
	}
	for _, record := range m.writeWorkflowRun.NativeTestRegistrations {
		if record.PlanID == plan.ID {
			return fmt.Errorf("registration plan ID already used")
		}
	}
	*plan = candidate
	m.writeWorkflowRun.NativeTestRegistrations = append(m.writeWorkflowRun.NativeTestRegistrations, NativeTestRegistrationRecord{PlanID: plan.ID, Digest: digest})
	m.changePlan = plan
	m.partialChangePlan = nil
	m.nativeTestRegistrationAuthorization = nil
	return nil
}

func (m *MutableState) AuthorizeNativeTestRegistrationExecution(plan *ChangePlan, root string, delivery VerificationDeliverySnapshot, contracts []WriteBehaviorContract) error {
	if m == nil {
		return fmt.Errorf("missing execution context")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nativeTestRegistrationExecution = nil
	if NativeTestRegistrationDigest(plan) == "" || NativeTestRegistrationDigest(plan) != NativeTestRegistrationDigest(m.changePlan) || !nativeRegistrationExecutionState(plan, m.writeWorkflowRun) || !NativeTestRegistrationMatchesRun(plan, m.writeWorkflowRun, root) ||
		plan.NativeTestRegistration.AnalysisDigest != nativeRegistrationHash(m.writeAnalysisIR) ||
		nativeRegistrationHash(plan.NativeTestRegistration.Delivery) != nativeRegistrationHash(delivery) ||
		nativeRegistrationHash(plan.BehaviorContracts) != nativeRegistrationHash(contracts) {
		return fmt.Errorf("registration no longer matches the current source delivery and contracts")
	}
	m.nativeTestRegistrationExecution = &nativeTestRegistrationExecution{NativeTestRegistrationDigest(plan), root, m.nativeRegistrationContextDigestLocked()}
	return nil
}

func (m *MutableState) NativeTestRegistrationExecutionAuthorized(plan *ChangePlan, root string) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	g := m.nativeTestRegistrationExecution
	return g != nil && g.root == root && g.digest != "" && g.digest == NativeTestRegistrationDigest(plan) &&
		g.digest == NativeTestRegistrationDigest(m.changePlan) &&
		g.contextDigest != "" && g.contextDigest == m.nativeRegistrationContextDigestLocked() && nativeRegistrationExecutionState(plan, m.writeWorkflowRun) && NativeTestRegistrationMatchesRun(plan, m.writeWorkflowRun, root)
}

func nativeRegistrationExecutionState(plan *ChangePlan, run *WriteWorkflowRun) bool {
	if plan == nil || plan.Status == PlanStatusRejected || run == nil || run.Status != WriteWorkflowRunInProgress {
		return false
	}
	b := nativeRegistrationProofBatch(run)
	return b != nil && (b.Status == WriteWorkflowBatchPlanned || b.Status == WriteWorkflowBatchVerifying)
}

func (m *MutableState) RevokeNativeTestRegistrationExecution() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nativeTestRegistrationExecution = nil
}
