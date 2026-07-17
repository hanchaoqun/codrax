package tracequery

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"unsafe"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

const (
	perfIdentityAliasProjectionCap = 8
	// Prepaid by traceIndexCacheCost for every retained perf row. This bounds
	// the lazy ledger's sparse ordinal binding, worst-case one-row cohort,
	// private map entries and bounded alias projection without charging
	// zero-perf traces.
	perfIdentityLedgerReservedBytesPerSample int64 = 512
	// perfIdentityFullAliasCap bounds the private selector authority. Unlike
	// the public projection, reaching this cap is not allowed to silently drop
	// an alias: the whole cohort is withheld so no old comm can become a false
	// negative selector merely because it arrived after a display budget.
	perfIdentityFullAliasCap = 256
	perfIdentityCaveatCap    = 16
)

// perfThreadKey is the sole hard key for perf thread aggregates. scope is a
// private frozen-capture namespace; comm and TGID must never enter equality.
type perfThreadKey struct {
	scope      string
	TID        int
	Generation int
}

type perfThreadIdentityRecord struct {
	key             perfThreadKey
	identity        PerfThreadIdentity
	selectorAliases []string
	aliasTruncated  bool
}

type perfIdentityOrdinalBinding struct {
	ordinal int
	record  int
}

type perfIdentityCandidateRecord struct {
	ordinal int
	tid     int
	aliasA  string
	aliasB  string
}

type perfIdentitySelectorVerdict uint8

const (
	perfIdentitySelectorNoMatch perfIdentitySelectorVerdict = iota
	perfIdentitySelectorMatch
	perfIdentitySelectorWithheld
)

// perfIdentityLedger is immutable after ensurePerfIdentityLedger publishes
// it through Index.perfIdentityOnce. records is cohort-compact and bindings is
// sparse by perf event ordinal; a trace with zero perf rows allocates neither.
// Candidate records retain only the minimum accepted numeric/alias projection
// needed to distinguish selector no-match from identity-withheld.
type perfIdentityLedger struct {
	records    []perfThreadIdentityRecord
	bindings   []perfIdentityOrdinalBinding
	candidates []perfIdentityCandidateRecord
	caveatV1   []string
}

type perfIdentityScopePlan struct {
	causalSources  []int
	sharedScope    string
	sourceScopes   map[int]string
	sharedCapture  bool
	unboundSources bool
}

type perfIdentityCandidate struct {
	ordinal     int
	sourceIndex int
	scope       string
	tid         int
	tgid        int
	displayComm string
	aliasA      string
	aliasB      string
	key         perfThreadKey
}

type perfGenerationOrderedEvent struct {
	ordinal        int
	candidateIndex int
}

type perfIdentityCohort struct {
	tgid          int
	tgidConflict  bool
	displayComm   string
	displayTs     float64
	displayLine   int
	displaySet    bool
	aliasA        string
	aliasB        string
	aliasExtra    []string
	aliasOverflow bool
}

type perfIdentityCaveatBuilder struct {
	items     []string
	compacted bool
	seen      map[string]struct{}
}

func ensurePerfIdentityLedger(idx *Index) *perfIdentityLedger {
	if idx == nil {
		return &perfIdentityLedger{}
	}
	idx.perfIdentityOnce.Do(func() {
		idx.perfIdentity, _ = buildPerfIdentityLedgerContext(context.Background(), idx)
	})
	if idx.perfIdentity == nil {
		return &perfIdentityLedger{}
	}
	return idx.perfIdentity
}

// prebuildPerfIdentityLedger runs while a freshly parsed Index is still
// private to its builder. The cancellable build deliberately happens OUTSIDE
// sync.Once: a canceled attempt must not poison the Once and make a later
// healthy caller observe a permanently nil ledger. Only successful complete
// construction consumes the Once and publishes the immutable pointer.
func prebuildPerfIdentityLedger(ctx context.Context, idx *Index) error {
	if idx == nil {
		return nil
	}
	if idx.perfIdentity != nil {
		return nil
	}
	built, err := buildPerfIdentityLedgerContext(ctx, idx)
	if err != nil {
		return err
	}
	idx.perfIdentityOnce.Do(func() {
		idx.perfIdentity = built
	})
	if idx.perfIdentity == nil {
		return fmt.Errorf("perf identity ledger publication was consumed without a complete value")
	}
	return nil
}

const perfIdentityBuildCancelMask = (1 << 12) - 1

func perfIdentityBuildContextErr(ctx context.Context, ordinal int) error {
	if ctx == nil || ordinal&perfIdentityBuildCancelMask != 0 {
		return nil
	}
	return ctx.Err()
}

func (l *perfIdentityLedger) identityForEventOrdinal(ordinal int) (perfThreadKey, PerfThreadIdentity, bool) {
	key, identity, ok := l.identityForEventOrdinalBorrowed(ordinal)
	if !ok {
		return perfThreadKey{}, PerfThreadIdentity{}, false
	}
	identity.CommAliases = append([]string(nil), identity.CommAliases...)
	return key, identity, true
}

// identityForEventOrdinalBorrowed returns an immutable view owned by the
// ledger. Hot query loops must use this form: cloning CommAliases once per
// sample turns a dense 250k-sample profile into 250k avoidable allocations.
// Callers must never mutate the returned identity or its CommAliases slice.
func (l *perfIdentityLedger) identityForEventOrdinalBorrowed(ordinal int) (perfThreadKey, PerfThreadIdentity, bool) {
	if l == nil || ordinal < 0 || len(l.bindings) == 0 {
		return perfThreadKey{}, PerfThreadIdentity{}, false
	}
	i := sort.Search(len(l.bindings), func(i int) bool { return l.bindings[i].ordinal >= ordinal })
	if i >= len(l.bindings) || l.bindings[i].ordinal != ordinal || l.bindings[i].record < 0 || l.bindings[i].record >= len(l.records) {
		return perfThreadKey{}, PerfThreadIdentity{}, false
	}
	record := l.records[l.bindings[i].record]
	return record.key, record.identity, true
}

func (l *perfIdentityLedger) caveats() []string {
	if l == nil {
		return nil
	}
	return append([]string(nil), l.caveatV1...)
}

// retainedBytes returns a conservative count of heap storage owned by the
// immutable ledger (not temporary build maps/slices and not string payloads
// already owned/accounted by Index.Events). It deliberately counts both alias
// slice backings even when the short public projection shares the selector
// backing, so the LRU reserve invariant fails safely on representation drift.
func (l *perfIdentityLedger) retainedBytes() int64 {
	if l == nil {
		return 0
	}
	bytes := int64(unsafe.Sizeof(*l))
	bytes += int64(cap(l.records)) * int64(unsafe.Sizeof(perfThreadIdentityRecord{}))
	bytes += int64(cap(l.bindings)) * int64(unsafe.Sizeof(perfIdentityOrdinalBinding{}))
	bytes += int64(cap(l.candidates)) * int64(unsafe.Sizeof(perfIdentityCandidateRecord{}))
	for i := range l.records {
		record := &l.records[i]
		bytes += int64(cap(record.selectorAliases)) * int64(unsafe.Sizeof(""))
		bytes += int64(cap(record.identity.CommAliases)) * int64(unsafe.Sizeof(""))
		bytes += int64(len(record.key.scope))
	}
	bytes += int64(cap(l.caveatV1)) * int64(unsafe.Sizeof(""))
	for _, caveat := range l.caveatV1 {
		bytes += int64(len(caveat))
	}
	return bytes
}

// matchesComm checks exact comm equality against the complete private alias
// authority. The public CommAliases field is only a display projection and
// must never be used for selection.
func (l *perfIdentityLedger) matchesComm(key perfThreadKey, want string) bool {
	want = strings.TrimSpace(want)
	record, ok := l.recordForKey(key)
	if !ok || want == "" || record.aliasTruncated {
		return false
	}
	for _, alias := range record.selectorAliases {
		if strings.EqualFold(strings.TrimSpace(alias), want) {
			return true
		}
	}
	return false
}

// matchesThreadSelector applies the package's established selector semantics
// to the complete private alias set. A precise PID is checked first; a
// PID-only selector needs no comm match, while a PID+name selector must prove
// both dimensions against the same typed cohort.
func (l *perfIdentityLedger) matchesThreadSelector(key perfThreadKey, selector threadSelector) bool {
	if l == nil || key.TID <= 0 {
		return false
	}
	if selector.HasPID && selector.PID != key.TID {
		return false
	}
	if selector.HasPID {
		// A positive numeric TID is the precise selector authority. A name
		// suffix is UI/display context and must never veto the same typed cohort.
		return true
	}
	record, ok := l.recordForKey(key)
	if !ok || record.aliasTruncated {
		return false
	}
	for _, alias := range record.selectorAliases {
		if threadSelectorMatchesName(selector, alias) {
			return true
		}
	}
	return false
}

func (l *perfIdentityLedger) aliasesForKey(key perfThreadKey) []string {
	record, ok := l.recordForKey(key)
	if !ok {
		return nil
	}
	return append([]string(nil), record.selectorAliases...)
}

func (l *perfIdentityLedger) recordForKey(key perfThreadKey) (*perfThreadIdentityRecord, bool) {
	if l == nil || key.TID <= 0 || key.Generation <= 0 || len(l.records) == 0 {
		return nil, false
	}
	i := sort.Search(len(l.records), func(i int) bool {
		return !perfThreadKeyLess(l.records[i].key, key)
	})
	if i >= len(l.records) || l.records[i].key != key {
		return nil, false
	}
	return &l.records[i], true
}

func perfThreadKeyLess(left, right perfThreadKey) bool {
	if left.scope != right.scope {
		return left.scope < right.scope
	}
	if left.TID != right.TID {
		return left.TID < right.TID
	}
	return left.Generation < right.Generation
}

// selectorVerdictForEventOrdinal distinguishes a genuine no-match from an
// accepted numeric candidate whose generation/TGID/alias authority was
// deliberately withheld. Identity-addressed views use Withheld to fail closed
// instead of misreporting "no samples".
func (l *perfIdentityLedger) selectorVerdictForEventOrdinal(ordinal int, selector threadSelector) perfIdentitySelectorVerdict {
	if l == nil || ordinal < 0 {
		return perfIdentitySelectorNoMatch
	}
	key, identity, identityOK := l.identityForEventOrdinalBorrowed(ordinal)
	if identityOK {
		if selector.HasPID {
			if selector.PID == identity.TID {
				return perfIdentitySelectorMatch
			}
			return perfIdentitySelectorNoMatch
		}
		record, recordOK := l.recordForKey(key)
		if !recordOK {
			return perfIdentitySelectorWithheld
		}
		if record.aliasTruncated {
			return perfIdentitySelectorWithheld
		}
		if l.matchesThreadSelector(key, selector) {
			return perfIdentitySelectorMatch
		}
		return perfIdentitySelectorNoMatch
	}
	i := sort.Search(len(l.candidates), func(i int) bool { return l.candidates[i].ordinal >= ordinal })
	if i >= len(l.candidates) || l.candidates[i].ordinal != ordinal {
		return perfIdentitySelectorNoMatch
	}
	candidate := l.candidates[i]
	if selector.HasPID {
		if selector.PID != candidate.tid {
			return perfIdentitySelectorNoMatch
		}
		return perfIdentitySelectorWithheld
	}
	if strings.TrimSpace(selector.Name) == "" || threadSelectorMatchesName(selector, candidate.aliasA) || threadSelectorMatchesName(selector, candidate.aliasB) {
		return perfIdentitySelectorWithheld
	}
	return perfIdentitySelectorNoMatch
}

func buildPerfIdentityLedgerContext(ctx context.Context, idx *Index) (*perfIdentityLedger, error) {
	ledger := &perfIdentityLedger{}
	if idx == nil || len(idx.Events) == 0 {
		return ledger, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	plan := buildPerfIdentityScopePlan(idx)
	caveats := &perfIdentityCaveatBuilder{}
	candidateCapacity := 0
	for ordinal := range idx.Events {
		if err := perfIdentityBuildContextErr(ctx, ordinal); err != nil {
			return nil, err
		}
		if _, admitted := perfIdentityCandidateTIDFromEvent(idx.Events[ordinal]); admitted {
			candidateCapacity++
		}
	}

	// Candidate admission is deliberately independent of comm: only a
	// positive, internally consistent TID can enter the generation ledger.
	// Intentional source-only/zero rows remain anonymous inventory and do not
	// poison a healthy sibling.
	candidates := make([]perfIdentityCandidate, 0, candidateCapacity)
	invalidScopeTID := map[perfThreadScopeTID]bool{}
	invalidAllScopesTID := map[int]bool{}
	for ordinal := range idx.Events {
		if err := perfIdentityBuildContextErr(ctx, ordinal); err != nil {
			return nil, err
		}
		ev := idx.Events[ordinal]
		candidate, tidsAtRisk, admitted := perfIdentityCandidateFromEvent(ev, ordinal)
		if len(tidsAtRisk) == 0 && !admitted {
			continue
		}
		scope, sourceIndex, scoped := plan.scopeForEvent(idx, ev)
		if !scoped {
			for _, tid := range tidsAtRisk {
				if tid > 0 {
					invalidAllScopesTID[tid] = true
				}
			}
			if admitted {
				invalidAllScopesTID[candidate.tid] = true
				ledger.candidates = append(ledger.candidates, perfIdentityCandidateSidecar(candidate))
				caveats.add(fmt.Sprintf("perf_thread_identity_source_unresolved=true; tid=%d", candidate.tid))
			}
			continue
		}
		for _, tid := range tidsAtRisk {
			if tid > 0 {
				invalidScopeTID[perfThreadScopeTID{scope: scope, tid: tid}] = true
			}
		}
		if !admitted {
			if len(tidsAtRisk) > 0 {
				caveats.add(fmt.Sprintf("perf_thread_identity_inconsistent=true; scope=%s tid=%d", scope, tidsAtRisk[0]))
			}
			continue
		}
		candidate.scope = scope
		candidate.sourceIndex = sourceIndex
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		ledger.caveatV1 = caveats.finish()
		return ledger, nil
	}
	// A capped preserved lifecycle audit has lost the identity of one or more
	// omitted physical boundaries. It cannot be scoped to a contributor, so
	// every numeric perf candidate fails closed while anonymous inventory
	// remains unaffected. Normal generation assignment below is otherwise
	// candidate-scoped and never materializes the global scheduler roster.
	if idx.threadIncarnationFailuresCapped {
		caveats.add("perf_thread_generation_audit_capped=true; numeric_thread_attribution_withheld=true")
		ledger.candidates = make([]perfIdentityCandidateRecord, 0, len(candidates))
		for i := range candidates {
			if err := perfIdentityBuildContextErr(ctx, i); err != nil {
				return nil, err
			}
			ledger.candidates = append(ledger.candidates, perfIdentityCandidateSidecar(candidates[i]))
		}
		ledger.caveatV1 = caveats.finish()
		return ledger, nil
	}

	if err := markUnboundCrossSourceTIDs(ctx, plan, candidates, invalidAllScopesTID, caveats); err != nil {
		return nil, err
	}
	if err := assignPerfCandidateGenerations(ctx, idx, plan, candidates, invalidScopeTID, invalidAllScopesTID, caveats); err != nil {
		return nil, err
	}

	cohortIndex := make(map[perfThreadKey]int, len(candidates))
	cohorts := make([]perfIdentityCohort, 0, len(candidates))
	for i := range candidates {
		if err := perfIdentityBuildContextErr(ctx, i); err != nil {
			return nil, err
		}
		candidate := &candidates[i]
		if invalidAllScopesTID[candidate.tid] || invalidScopeTID[perfThreadScopeTID{scope: candidate.scope, tid: candidate.tid}] {
			continue
		}
		if candidate.key.TID <= 0 || candidate.key.Generation <= 0 {
			invalidScopeTID[perfThreadScopeTID{scope: candidate.scope, tid: candidate.tid}] = true
			caveats.add(fmt.Sprintf("perf_thread_generation_unorderable=true; scope=%s tid=%d", candidate.scope, candidate.tid))
			continue
		}
		index, exists := cohortIndex[candidate.key]
		if !exists {
			index = len(cohorts)
			cohortIndex[candidate.key] = index
			cohorts = append(cohorts, perfIdentityCohort{})
		}
		cohort := &cohorts[index]
		if candidate.tgid > 0 {
			if cohort.tgid > 0 && cohort.tgid != candidate.tgid {
				cohort.tgidConflict = true
			} else {
				cohort.tgid = candidate.tgid
			}
		}
		appendPerfIdentityCohortAlias(cohort, candidate.aliasA)
		appendPerfIdentityCohortAlias(cohort, candidate.aliasB)
		if candidate.displayComm != "" {
			ev := idx.Events[candidate.ordinal]
			latest := !cohort.displaySet
			if plan.sharedCapture {
				latest = latest || ev.Ts > cohort.displayTs || ev.Ts == cohort.displayTs && ev.Line > cohort.displayLine
			} else {
				latest = latest || ev.Line > cohort.displayLine
			}
			if latest {
				cohort.displayComm = candidate.displayComm
				cohort.displayTs = ev.Ts
				cohort.displayLine = ev.Line
				cohort.displaySet = true
			}
		}
	}

	// A late unorderable sample invalidates all already-built generations for
	// that scope/TID. This second pass is what keeps an ambiguous row from
	// being silently dropped while healthy siblings appear authoritative.
	cohortKeys := make([]perfThreadKey, 0, len(cohortIndex))
	cohortKeyOrdinal := 0
	for key := range cohortIndex {
		if err := perfIdentityBuildContextErr(ctx, cohortKeyOrdinal); err != nil {
			return nil, err
		}
		cohortKeys = append(cohortKeys, key)
		cohortKeyOrdinal++
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(cohortKeys, func(i, j int) bool {
		if cohortKeys[i].scope != cohortKeys[j].scope {
			return cohortKeys[i].scope < cohortKeys[j].scope
		}
		if cohortKeys[i].TID != cohortKeys[j].TID {
			return cohortKeys[i].TID < cohortKeys[j].TID
		}
		return cohortKeys[i].Generation < cohortKeys[j].Generation
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ledger.records = make([]perfThreadIdentityRecord, 0, len(cohortKeys))
	for _, key := range cohortKeys {
		if err := perfIdentityBuildContextErr(ctx, len(ledger.records)); err != nil {
			return nil, err
		}
		cohort := &cohorts[cohortIndex[key]]
		if invalidAllScopesTID[key.TID] || invalidScopeTID[perfThreadScopeTID{scope: key.scope, tid: key.TID}] {
			continue
		}
		if cohort.tgidConflict {
			caveats.add(fmt.Sprintf("perf_thread_tgid_conflict=true; scope=%s tid=%d generation=%d", key.scope, key.TID, key.Generation))
			continue
		}
		identity := PerfThreadIdentity{
			TID:         key.TID,
			TGID:        cohort.tgid,
			Generation:  key.Generation,
			DisplayComm: cohort.displayComm,
		}
		fullAliases := sortedPerfIdentityCohortAliases(cohort)
		if cohort.aliasOverflow {
			identity.CommAliasCountAtLeast = perfIdentityFullAliasCap + 1
			identity.CommAliasesTruncated = true
			caveats.add(fmt.Sprintf("perf_thread_alias_authority_capped=true; scope=%s tid=%d generation=%d cap=%d; comm_selector_withheld=true", key.scope, key.TID, key.Generation, perfIdentityFullAliasCap))
		} else {
			identity.CommAliasCount = len(fullAliases)
		}
		identity.CommAliases = projectPerfIdentityAliases(fullAliases, cohort.displayComm)
		ledger.records = append(ledger.records, perfThreadIdentityRecord{
			key: key, identity: identity, selectorAliases: fullAliases, aliasTruncated: cohort.aliasOverflow,
		})
	}
	// Records are emitted in perfThreadKey order. Resolve every accepted
	// candidate through that compact immutable table instead of retaining an
	// ordinal slice in every cohort (one tiny heap object per TID on a high-
	// cardinality profile).
	ledger.bindings = make([]perfIdentityOrdinalBinding, 0, len(candidates))
	for i := range candidates {
		if err := perfIdentityBuildContextErr(ctx, i); err != nil {
			return nil, err
		}
		candidate := &candidates[i]
		if candidate.key.TID <= 0 || candidate.key.Generation <= 0 {
			continue
		}
		recordIndex := sort.Search(len(ledger.records), func(i int) bool {
			return !perfThreadKeyLess(ledger.records[i].key, candidate.key)
		})
		if recordIndex < len(ledger.records) && ledger.records[recordIndex].key == candidate.key {
			ledger.bindings = append(ledger.bindings, perfIdentityOrdinalBinding{ordinal: candidate.ordinal, record: recordIndex})
		}
	}
	// candidates are admitted in event-ordinal order, so bindings are already
	// sorted; a second 250k-row sort would add cancel tail without information.
	// The sidecar exists only for admitted candidates that lack a published
	// binding. Successful dense perf traces therefore pay one sparse ordinal
	// binding, not a duplicate candidate record for every sample.
	withheldCapacity := len(ledger.candidates) + len(candidates) - len(ledger.bindings)
	withheld := make([]perfIdentityCandidateRecord, len(ledger.candidates), withheldCapacity)
	copy(withheld, ledger.candidates)
	ledger.candidates = withheld
	bound := 0
	for i := range candidates {
		if err := perfIdentityBuildContextErr(ctx, i); err != nil {
			return nil, err
		}
		ordinal := candidates[i].ordinal
		for bound < len(ledger.bindings) && ledger.bindings[bound].ordinal < ordinal {
			bound++
		}
		if bound >= len(ledger.bindings) || ledger.bindings[bound].ordinal != ordinal {
			ledger.candidates = append(ledger.candidates, perfIdentityCandidateSidecar(candidates[i]))
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(ledger.candidates, func(i, j int) bool { return ledger.candidates[i].ordinal < ledger.candidates[j].ordinal })
	ledger.caveatV1 = caveats.finish()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ledger, nil
}

func perfIdentityCandidateSidecar(candidate perfIdentityCandidate) perfIdentityCandidateRecord {
	record := perfIdentityCandidateRecord{ordinal: candidate.ordinal, tid: candidate.tid}
	record.aliasA = candidate.aliasA
	record.aliasB = candidate.aliasB
	return record
}

func appendPerfIdentityCohortAlias(cohort *perfIdentityCohort, alias string) {
	if cohort == nil {
		return
	}
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return
	}
	if cohort.aliasA == alias || cohort.aliasB == alias {
		return
	}
	for _, existing := range cohort.aliasExtra {
		if existing == alias {
			return
		}
	}
	count := len(cohort.aliasExtra)
	if cohort.aliasA != "" {
		count++
	}
	if cohort.aliasB != "" {
		count++
	}
	if count >= perfIdentityFullAliasCap {
		cohort.aliasOverflow = true
		return
	}
	if cohort.aliasA == "" {
		cohort.aliasA = alias
		return
	}
	if cohort.aliasB == "" {
		cohort.aliasB = alias
		return
	}
	cohort.aliasExtra = append(cohort.aliasExtra, alias)
}

func sortedPerfIdentityCohortAliases(cohort *perfIdentityCohort) []string {
	if cohort == nil || cohort.aliasA == "" {
		return nil
	}
	out := make([]string, 0, 2+len(cohort.aliasExtra))
	out = append(out, cohort.aliasA)
	if cohort.aliasB != "" {
		out = append(out, cohort.aliasB)
	}
	out = append(out, cohort.aliasExtra...)
	sort.Strings(out)
	return out
}

type perfThreadScopeTID struct {
	scope string
	tid   int
}

func buildPerfIdentityScopePlan(idx *Index) perfIdentityScopePlan {
	plan := perfIdentityScopePlan{sourceScopes: map[int]string{}}
	if idx == nil || len(idx.TraceArtifacts) == 0 {
		plan.sharedScope = "capture:implicit"
		return plan
	}
	for i := range idx.TraceArtifacts {
		if idx.TraceArtifacts[i].CausalCompatible {
			plan.causalSources = append(plan.causalSources, i)
		}
	}
	if len(plan.causalSources) <= 1 {
		plan.sharedScope = "capture:implicit"
		return plan
	}
	captureID := ""
	shared := true
	for _, sourceIndex := range plan.causalSources {
		source := &idx.TraceArtifacts[sourceIndex]
		currentCaptureID := strings.TrimSpace(source.CaptureID)
		if source.BundleSchema != tracebundle.SchemaV2 || currentCaptureID == "" {
			shared = false
			break
		}
		if captureID == "" {
			captureID = currentCaptureID
		} else if captureID != currentCaptureID {
			shared = false
			break
		}
	}
	if shared && captureID != "" {
		plan.sharedCapture = true
		plan.sharedScope = "capture:" + captureID
		return plan
	}
	plan.unboundSources = true
	for _, sourceIndex := range plan.causalSources {
		plan.sourceScopes[sourceIndex] = fmt.Sprintf("artifact:%d", sourceIndex)
	}
	return plan
}

func (p perfIdentityScopePlan) scopeForEvent(idx *Index, ev Event) (string, int, bool) {
	if idx == nil {
		return "", -1, false
	}
	if len(idx.TraceArtifacts) == 0 {
		return p.sharedScope, -1, p.sharedScope != ""
	}
	if len(p.causalSources) == 1 {
		// A single causal artifact is an implicit capture universe. Its source
		// range may be absent on a one-artifact synthetic/additive caller. If
		// isolated siblings exist, however, the event must still resolve into
		// the sole causal source; otherwise an impossible isolated child row
		// could inherit the implicit scope.
		if len(idx.TraceArtifacts) == 1 {
			return p.sharedScope, p.causalSources[0], p.sharedScope != ""
		}
		sourceIndex, ok := resolveTraceArtifactSourceIndexForLine(idx.TraceArtifacts, ev.Line)
		if !ok || sourceIndex != p.causalSources[0] {
			return "", -1, false
		}
		return p.sharedScope, sourceIndex, p.sharedScope != ""
	}
	sourceIndex, ok := resolveTraceArtifactSourceIndexForLine(idx.TraceArtifacts, ev.Line)
	if !ok {
		return "", -1, false
	}
	if p.sharedCapture {
		return p.sharedScope, sourceIndex, true
	}
	scope, ok := p.sourceScopes[sourceIndex]
	return scope, sourceIndex, ok
}

func perfIdentityCandidateFromEvent(ev Event, ordinal int) (perfIdentityCandidate, []int, bool) {
	tid, admitted := perfIdentityCandidateTIDFromEvent(ev)
	if !admitted {
		return perfIdentityCandidate{}, nil, false
	}
	pf := ev.PerfFields
	if pf == nil {
		pf = &PerfFields{}
	}
	// The normalized perf body is authoritative when present; Event.PID/TGID
	// are compatibility fallbacks for legacy/synthetic producers. The parser's
	// envelope/body integrity gate has already withdrawn a contradictory wire
	// row, so re-comparing the two here would reinterpret a transport header as
	// a second thread claim and break the established pf.TID precedence.
	tgid := pf.PID
	if tgid <= 0 {
		tgid = ev.TGID
	}
	if tgid <= 0 {
		tgid = 0
	}
	aliasA := strings.TrimSpace(pf.Comm)
	aliasB := strings.TrimSpace(ev.Comm)
	if aliasA == "" {
		aliasA, aliasB = aliasB, ""
	} else if aliasB == aliasA {
		aliasB = ""
	}
	return perfIdentityCandidate{
		ordinal:     ordinal,
		tid:         tid,
		tgid:        tgid,
		displayComm: strings.TrimSpace(firstNonEmpty(pf.Comm, ev.Comm)),
		aliasA:      aliasA,
		aliasB:      aliasB,
	}, nil, true
}

// perfIdentityCandidateTIDFromEvent is the zero-allocation numeric admission
// primitive used by generation scans. Alias projection belongs only to the
// ledger's first normalization pass and must not allocate again for every
// warm/cold/retained lifecycle lookup.
func perfIdentityCandidateTIDFromEvent(ev Event) (int, bool) {
	if ev.Type != EventPerfSample || perfSampleIsSourceOnlyIdentity(ev) || !perfSampleHasTypedThreadIdentity(ev) {
		return 0, false
	}
	pf := ev.PerfFields
	if pf == nil {
		pf = &PerfFields{}
	}
	tid := firstNonZero(pf.TID, ev.PID)
	return tid, tid > 0
}

func markUnboundCrossSourceTIDs(ctx context.Context, plan perfIdentityScopePlan, candidates []perfIdentityCandidate, invalid map[int]bool, caveats *perfIdentityCaveatBuilder) error {
	if !plan.unboundSources {
		return nil
	}
	type sourceSet struct {
		first  int
		extra  map[int]struct{}
		count  int
		seeded bool
	}
	sourcesByTID := map[int]sourceSet{}
	for i, candidate := range candidates {
		if err := perfIdentityBuildContextErr(ctx, i); err != nil {
			return err
		}
		set := sourcesByTID[candidate.tid]
		if !set.seeded {
			set.first = candidate.sourceIndex
			set.count = 1
			set.seeded = true
			sourcesByTID[candidate.tid] = set
			continue
		}
		if candidate.sourceIndex == set.first {
			continue
		}
		if set.extra == nil {
			set.extra = map[int]struct{}{}
		}
		if _, exists := set.extra[candidate.sourceIndex]; !exists {
			set.extra[candidate.sourceIndex] = struct{}{}
			set.count++
		}
		sourcesByTID[candidate.tid] = set
	}
	secondPassOrdinal := 0
	for tid, sources := range sourcesByTID {
		if err := perfIdentityBuildContextErr(ctx, secondPassOrdinal); err != nil {
			return err
		}
		secondPassOrdinal++
		if sources.count <= 1 {
			continue
		}
		invalid[tid] = true
		caveats.add(fmt.Sprintf("perf_thread_cross_source_unbound=true; tid=%d sources=%d", tid, sources.count))
	}
	return ctx.Err()
}

// assignPerfCandidateGenerations replays only lifecycle rows that can affect
// an admitted numeric perf candidate. This is the generation single authority
// for the perf ledger: window-head checkpoints provide the exact prefix state,
// then retained events advance it in the governing order. Unbound artifacts
// replay independently in physical-line order; a verified V2 capture replays
// once in canonical timestamp+virtual-line order.
func assignPerfCandidateGenerations(ctx context.Context, idx *Index, plan perfIdentityScopePlan, candidates []perfIdentityCandidate, invalidScoped map[perfThreadScopeTID]bool, invalidAll map[int]bool, caveats *perfIdentityCaveatBuilder) error {
	if idx == nil || len(candidates) == 0 {
		return nil
	}
	pidsByScope := map[string]map[int]bool{}
	for i := range candidates {
		if err := perfIdentityBuildContextErr(ctx, i); err != nil {
			return err
		}
		candidate := &candidates[i]
		set := pidsByScope[candidate.scope]
		if set == nil {
			set = map[int]bool{}
			pidsByScope[candidate.scope] = set
		}
		set[candidate.tid] = true
	}
	if err := markRetainedPerfGenerationIntegrityFailures(ctx, idx, plan, pidsByScope, invalidScoped, caveats); err != nil {
		return err
	}

	orders := make(map[string][]perfGenerationOrderedEvent, len(pidsByScope))
	candidateCursor := 0
	for ordinal := range idx.Events {
		if err := perfIdentityBuildContextErr(ctx, ordinal); err != nil {
			return err
		}
		ev := idx.Events[ordinal]
		for candidateCursor < len(candidates) && candidates[candidateCursor].ordinal < ordinal {
			candidateCursor++
		}
		candidateIndex := -1
		if candidateCursor < len(candidates) && candidates[candidateCursor].ordinal == ordinal {
			candidateIndex = candidateCursor
		}
		candidateEvent := candidateIndex >= 0
		scope, sourceIndex, ok := plan.scopeForEvent(idx, ev)
		if !ok {
			if candidateEvent {
				invalidAll[candidates[candidateIndex].tid] = true
			}
			continue
		}
		pids := pidsByScope[scope]
		if len(pids) == 0 {
			continue
		}
		touched := perfGenerationEventCandidateTIDs(ev, pids)
		if candidateEvent {
			touched.add(candidates[candidateIndex].tid, pids)
		}
		if touched.count == 0 {
			continue
		}
		// A canonical merge cannot repair a physical source whose own clock
		// moved backwards: sorting would silently invent lifecycle order. Poison
		// only candidate TIDs actually touched by that source, preserving useful
		// identities from independent siblings in the same bundle.
		if perfGenerationSourceOrderUnproven(idx, sourceIndex) {
			for i := 0; i < touched.count; i++ {
				tid := touched.values[i]
				invalidScoped[perfThreadScopeTID{scope: scope, tid: tid}] = true
				caveats.add(fmt.Sprintf("perf_thread_event_order_unproven=true; scope=%s tid=%d reason=source_nonmonotonic", scope, tid))
			}
		}
		orders[scope] = append(orders[scope], perfGenerationOrderedEvent{ordinal: ordinal, candidateIndex: candidateIndex})
	}

	for scope, order := range orders {
		if err := ctx.Err(); err != nil {
			return err
		}
		pids := pidsByScope[scope]
		sort.SliceStable(order, func(i, j int) bool {
			left, right := idx.Events[order[i].ordinal], idx.Events[order[j].ordinal]
			if plan.sharedCapture && left.Ts != right.Ts {
				return left.Ts < right.Ts
			}
			return left.Line < right.Line
		})
		if plan.sharedCapture {
			if err := markRetainedSharedPerfGenerationSimultaneity(ctx, idx, plan, scope, order, candidates, pids, invalidScoped, caveats); err != nil {
				return err
			}
		}
		tracker := newPerfGenerationTracker()
		if head := idx.perfGenerationHeads[scope]; head != nil {
			tracker = head.cloneForPIDs(pids)
		}
		type coordinate struct {
			ts   uint64
			line int
		}
		var previousCoordinate coordinate
		previousOrdinal := -1
		previousCoordinateSet := false
		for i, ordered := range order {
			if err := perfIdentityBuildContextErr(ctx, i); err != nil {
				return err
			}
			ordinal := ordered.ordinal
			ev := idx.Events[ordinal]
			coord := coordinate{line: ev.Line}
			valid := ev.Line > 0
			if plan.sharedCapture {
				valid = valid && !math.IsNaN(ev.Ts) && !math.IsInf(ev.Ts, 0)
				coord.ts = math.Float64bits(ev.Ts)
			}
			if valid {
				if previousCoordinateSet && coord == previousCoordinate && previousOrdinal != ordinal {
					markPerfGenerationEventCandidatesInvalid(idx.Events[previousOrdinal], pids, scope, invalidScoped)
					markPerfGenerationEventCandidatesInvalid(ev, pids, scope, invalidScoped)
					valid = false
				}
				previousCoordinate, previousOrdinal, previousCoordinateSet = coord, ordinal, true
			}
			if !valid {
				markPerfGenerationEventCandidatesInvalid(ev, pids, scope, invalidScoped)
				caveats.add(fmt.Sprintf("perf_thread_event_order_unproven=true; scope=%s line=%d", scope, ev.Line))
				continue
			}
			candidateIndex, candidateEvent := ordered.candidateIndex, ordered.candidateIndex >= 0
			observed := ev
			if candidateEvent {
				// The typed perf body owns the sample TID. Event.PID may be a
				// compatibility envelope (or zero), so it must not become a
				// second generation authority.
				observed.PID = candidates[candidateIndex].tid
			}
			tracker.observeAllForPIDSet(observed, pids)
			if !candidateEvent {
				continue
			}
			candidate := &candidates[candidateIndex]
			key := perfThreadScopeTID{scope: scope, tid: candidate.tid}
			if reason := idx.perfGenerationHeadInvalid[key]; reason != "" {
				invalidScoped[key] = true
				caveats.add(fmt.Sprintf("perf_thread_generation_prefix_unproven=true; scope=%s tid=%d reason=%s", scope, candidate.tid, reason))
				continue
			}
			generation := tracker.generationForPID(candidate.tid)
			if generation <= 0 {
				invalidScoped[key] = true
				continue
			}
			candidate.key = perfThreadKey{scope: scope, TID: candidate.tid, Generation: generation}
		}
	}
	return ctx.Err()
}

func markRetainedSharedPerfGenerationSimultaneity(ctx context.Context, idx *Index, plan perfIdentityScopePlan, scope string, order []perfGenerationOrderedEvent, candidates []perfIdentityCandidate, pids map[int]bool, invalid map[perfThreadScopeTID]bool, caveats *perfIdentityCaveatBuilder) error {
	owners := map[int]int{}
	currentTs := uint64(0)
	currentTsSet := false
	for i, ordered := range order {
		if err := perfIdentityBuildContextErr(ctx, i); err != nil {
			return err
		}
		ordinal := ordered.ordinal
		ev := idx.Events[ordinal]
		tsBits := math.Float64bits(ev.Ts)
		if !currentTsSet || tsBits != currentTs {
			clear(owners)
			currentTs, currentTsSet = tsBits, true
		}
		_, sourceIndex, ok := plan.scopeForEvent(idx, ev)
		if !ok {
			continue
		}
		touched := perfGenerationEventCandidateTIDs(ev, pids)
		if ordered.candidateIndex >= 0 {
			touched.add(candidates[ordered.candidateIndex].tid, pids)
		}
		for i := 0; i < touched.count; i++ {
			tid := touched.values[i]
			if priorSource, exists := owners[tid]; exists && priorSource != sourceIndex {
				invalid[perfThreadScopeTID{scope: scope, tid: tid}] = true
				caveats.add(fmt.Sprintf("perf_thread_event_order_unproven=true; scope=%s tid=%d reason=cross_artifact_simultaneous", scope, tid))
			} else {
				owners[tid] = sourceIndex
			}
		}
	}
	return ctx.Err()
}

func markRetainedPerfGenerationIntegrityFailures(ctx context.Context, idx *Index, plan perfIdentityScopePlan, pidsByScope map[string]map[int]bool, invalid map[perfThreadScopeTID]bool, caveats *perfIdentityCaveatBuilder) error {
	if idx == nil || len(pidsByScope) == 0 {
		return nil
	}
	mark := func(scope string, failure *schedulerRowIntegrityFailure) error {
		pids := pidsByScope[scope]
		if len(pids) == 0 {
			return nil
		}
		if failure == nil || failure.AffectsAllPIDs || len(failure.PIDs) == 0 {
			caveats.add(fmt.Sprintf("perf_thread_generation_scheduler_unproven=true; scope=%s affected_candidate_tids_at_least=%d reason=malformed_scheduler_identity", scope, len(pids)))
			ordinal := 0
			for tid := range pids {
				if err := perfIdentityBuildContextErr(ctx, ordinal); err != nil {
					return err
				}
				ordinal++
				invalid[perfThreadScopeTID{scope: scope, tid: tid}] = true
			}
			return nil
		}
		for _, tid := range failure.PIDs {
			if pids[tid] {
				invalid[perfThreadScopeTID{scope: scope, tid: tid}] = true
				caveats.add(fmt.Sprintf("perf_thread_generation_scheduler_unproven=true; scope=%s tid=%d reason=malformed_scheduler_identity", scope, tid))
			}
		}
		return nil
	}
	if idx.schedulerRowIntegrityFailuresCapped {
		for scope := range pidsByScope {
			if err := mark(scope, nil); err != nil {
				return err
			}
		}
	}
	for i := range idx.schedulerRowIntegrityFailures {
		failure := &idx.schedulerRowIntegrityFailures[i]
		scope, ok := perfGenerationScopeForSchedulerFailure(idx, plan, failure)
		if !ok {
			for candidateScope := range pidsByScope {
				if err := mark(candidateScope, failure); err != nil {
					return err
				}
			}
			continue
		}
		if err := mark(scope, failure); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func perfGenerationScopeForSchedulerFailure(idx *Index, plan perfIdentityScopePlan, failure *schedulerRowIntegrityFailure) (string, bool) {
	if idx == nil || failure == nil {
		return "", false
	}
	if plan.sharedScope != "" && !plan.unboundSources {
		return plan.sharedScope, true
	}
	if failure.SourcePath != "" {
		found := -1
		for sourceIndex := range idx.TraceArtifacts {
			source := &idx.TraceArtifacts[sourceIndex]
			if source.CausalCompatible && source.SourcePath == failure.SourcePath {
				if found >= 0 {
					return "", false
				}
				found = sourceIndex
			}
		}
		if found >= 0 {
			scope, ok := plan.sourceScopes[found]
			return scope, ok
		}
	}
	sourceIndex, ok := resolveTraceArtifactSourceIndexForLine(idx.TraceArtifacts, failure.Line)
	if !ok {
		return "", false
	}
	scope, ok := plan.sourceScopes[sourceIndex]
	return scope, ok
}

func markPerfGenerationEventCandidatesInvalid(ev Event, pids map[int]bool, scope string, invalid map[perfThreadScopeTID]bool) {
	touched := perfGenerationEventCandidateTIDs(ev, pids)
	for i := 0; i < touched.count; i++ {
		tid := touched.values[i]
		invalid[perfThreadScopeTID{scope: scope, tid: tid}] = true
	}
}

func projectPerfIdentityAliases(aliases []string, display string) []string {
	if len(aliases) == 0 {
		return nil
	}
	if len(aliases) <= perfIdentityAliasProjectionCap {
		return aliases
	}
	out := append([]string(nil), aliases[:perfIdentityAliasProjectionCap]...)
	display = strings.TrimSpace(display)
	if display != "" && !perfIdentityStringSliceContains(out, display) {
		out[len(out)-1] = display
		sort.Strings(out)
	}
	return out
}

func perfIdentityStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (b *perfIdentityCaveatBuilder) add(value string) {
	if b == nil || strings.TrimSpace(value) == "" {
		return
	}
	if b.seen == nil {
		b.seen = map[string]struct{}{}
	}
	if _, duplicate := b.seen[value]; duplicate {
		return
	}
	if len(b.items) < perfIdentityCaveatCap-1 {
		b.seen[value] = struct{}{}
		b.items = append(b.items, value)
		sort.Strings(b.items)
		return
	}
	// The display budget is also the retained-memory budget. Once compacted,
	// do not remember an attacker-controlled set of distinct invalid TIDs just
	// to compute an exact omitted count; publish the honest lower bound.
	b.compacted = true
	if value >= b.items[len(b.items)-1] {
		return
	}
	delete(b.seen, b.items[len(b.items)-1])
	b.items[len(b.items)-1] = value
	b.seen[value] = struct{}{}
	sort.Strings(b.items)
}

func (b *perfIdentityCaveatBuilder) finish() []string {
	if b == nil {
		return nil
	}
	out := append([]string(nil), b.items...)
	if b.compacted {
		out = append(out, "perf_thread_identity_caveats_compacted=true; omitted_at_least=1")
	}
	return out
}
