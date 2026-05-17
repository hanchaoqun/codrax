package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// violation_root_cause.go — W2.3 (2026-05-05).
//
// Root-cause closure: given a list of Violations from one contract
// check, group them by (kind, fingerprint) and use ViolKindSpec.Implies
// to mark symptom violations as derived. The retry hint builder
// then surfaces only the root violation per group, eliminating the
// "fix one, three new ones appear" rotation pattern observed in
// qfa-mr3 (docs/design/iteration_inflation_remediation.md §3 Source #1).
//
// Algorithm (per fingerprint group):
//   1. Walk Implies graph forward — A.Implies = [B] means "if A and
//      B fire on the same fingerprint, B is a symptom of A".
//   2. Within a fingerprint group, prefer kinds that imply others
//      (i.e. NOT pointed to by anyone in the group) as roots; mark
//      the rest IsDerived=true with RootKind=<root>.
//   3. Multiple independent roots in one fingerprint stay separate;
//      only direct Implies relations collapse.
//
// The closure is RUN-LOCAL: it does not mutate the registry, it
// produces a new []Violation slice with IsDerived / RootKind set.
// Concurrent-safe — pure function of input.

// ComputeRootCauseClosure returns a copy of `violations` where each
// element's IsDerived / RootKind fields are populated based on
// ViolKindSpec.Implies relations within the same (Kind, Fingerprint)
// fingerprint group.
//
// The fingerprint is derived from clusterFingerprintOf so it lines
// up with repair cluster identity. Violations whose Kind is not
// registered with a spec pass through unchanged.
//
// Output ordering preserves input ordering.
func ComputeRootCauseClosure(violations []types.Violation) []types.Violation {
	if len(violations) == 0 {
		return nil
	}
	out := make([]types.Violation, len(violations))
	copy(out, violations)

	// Group by fingerprint. Within each group, find which Kinds are
	// the "implier" (root) and which are "implied" (derived).
	type groupEntry struct {
		idx  int
		kind types.ViolationKind
	}
	groups := make(map[string][]groupEntry, len(out))
	for i, v := range out {
		fp := clusterFingerprintOf(v)
		groups[fp] = append(groups[fp], groupEntry{idx: i, kind: v.Kind})
	}

	for _, members := range groups {
		if len(members) < 2 {
			continue // singleton group, no derivation possible
		}
		// Build per-group kind set for fast membership check.
		kindSet := make(map[types.ViolationKind]bool, len(members))
		for _, m := range members {
			kindSet[m.kind] = true
		}
		// For each member, check if any OTHER member's Implies list
		// contains this kind. If yes, mark this one derived and
		// point RootKind to the implier.
		for _, m := range members {
			rootKind, derived := findRootInGroup(m.kind, kindSet)
			if !derived {
				continue
			}
			out[m.idx].IsDerived = true
			out[m.idx].RootKind = rootKind
		}
	}
	return out
}

// findRootInGroup walks the registry: for the given kind, return
// (rootKind, true) when any kind in groupKinds (other than kind
// itself) declares Implies containing kind. Otherwise return
// (zero, false).
//
// In case of multiple impliers, the first match in registry order
// wins — deterministic since AllViolKindSpecs is order-stable.
func findRootInGroup(kind types.ViolationKind, groupKinds map[types.ViolationKind]bool) (types.ViolationKind, bool) {
	for _, spec := range types.AllViolKindSpecs() {
		if spec.Kind == kind {
			continue
		}
		if !groupKinds[spec.Kind] {
			continue
		}
		for _, implied := range spec.Implies {
			if implied == kind {
				return spec.Kind, true
			}
		}
	}
	return "", false
}

// FilterDerivedViolations returns a copy of `violations` with
// IsDerived=true entries removed. Used by the retry hint builder
// so the LLM sees only root violations.
//
// Independent of ComputeRootCauseClosure — caller chooses whether
// to drop derived entries from the LLM-facing surface or merely
// suppress them in the retry hint while keeping them in telemetry.
func FilterDerivedViolations(violations []types.Violation) []types.Violation {
	if len(violations) == 0 {
		return nil
	}
	out := make([]types.Violation, 0, len(violations))
	for _, v := range violations {
		if v.IsDerived {
			continue
		}
		out = append(out, v)
	}
	return out
}

// FilterActionableRootViolations returns the subset that may drive
// retry / repair control flow: root-cause entries that are not
// permanently telemetry-only.
//
// Soft telemetry still belongs in the contract ledger and operator
// logs, but it must not be sent back to the LLM as a repair task.
// This is the central guard for permanently-soft kinds such as
// richness_regression and diagram_relation_label_only when they
// co-occur with one real hard failure.
func FilterActionableRootViolations(violations []types.Violation) []types.Violation {
	roots := FilterDerivedViolations(violations)
	if len(roots) == 0 {
		return nil
	}
	out := make([]types.Violation, 0, len(roots))
	for _, v := range roots {
		if isPermanentlyTelemetryOnlyViolationKind(v.Kind) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func isPermanentlyTelemetryOnlyViolationKind(k types.ViolationKind) bool {
	return !types.ViolationProfileFor(k, true).RetryEligible
}

// IncrementAttemptsAndCheckExhausted bumps the per-(kind, fp)
// counter on history for every ROOT violation in violations, then
// reports whether every root has reached MaxRepairAttemptsPerRoot.
// W2.6 (2026-05-05): cross-scope retry budget gate.
//
// "Every root exhausted" means we have nothing left to fix that
// retry could plausibly resolve — caller should ship the answer
// with materialised caveats instead of dispatching another retry.
//
// If history is nil (test paths / pre-W2 callers), behaviour is
// pre-W2: returns false unconditionally so the legacy retry budget
// continues to gate.
func IncrementAttemptsAndCheckExhausted(violations []types.Violation, history *types.RepairAttemptHistory) bool {
	if history == nil || len(violations) == 0 {
		return false
	}
	roots := FilterActionableRootViolations(violations)
	if len(roots) == 0 {
		return false
	}
	allExhausted := true
	for _, v := range roots {
		k := types.FpKey{Kind: v.Kind, Fingerprint: clusterFingerprintOf(v)}
		history.Increment(k)
		if !history.Exhausted(k) {
			allExhausted = false
		}
	}
	return allExhausted
}
