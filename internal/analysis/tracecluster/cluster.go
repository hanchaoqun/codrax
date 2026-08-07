// Package tracecluster turns verbose per-trace findings into stable,
// deterministic root-cause groups. It never reads raw trace timelines.
package tracecluster

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

const FingerprintVersion = "causal-v1"

var (
	instanceNumber = regexp.MustCompile(`\b(?:pid|tid|task|txn|transaction)[-_: =#]*\d+\b`)
	hexAddress     = regexp.MustCompile(`\b0x[0-9a-f]+\b`)
	timestamp      = regexp.MustCompile(`\b\d+(?:\.\d+)?(?:ns|us|µs|ms|s)\b`)
	anonymousIndex = regexp.MustCompile(`\b(?:thread|binder|worker)[-_: ]*\d+\b`)
	space          = regexp.MustCompile(`\s+`)
)

// NormalizeSignature removes instance-only details while retaining mechanism
// words. The result is safe to use in a semantic fingerprint.
func NormalizeSignature(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	v = hexAddress.ReplaceAllString(v, "<addr>")
	v = instanceNumber.ReplaceAllString(v, "<instance>")
	v = anonymousIndex.ReplaceAllString(v, "<instance>")
	v = timestamp.ReplaceAllString(v, "<duration>")
	return space.ReplaceAllString(v, " ")
}

func Fingerprint(decision types.TraceCauseDecision) types.TraceCauseFingerprintV1 {
	return types.TraceCauseFingerprintV1{
		Version: FingerprintVersion, Token: strings.TrimSpace(decision.Token.Token),
		Lane: strings.TrimSpace(decision.Token.Lane), SubjectRole: normalizeClosedValue(decision.SubjectRole),
		UpstreamRole: normalizeClosedValue(decision.UpstreamRole), CausalShape: strings.TrimSpace(decision.CausalShape),
		Phase: normalizeClosedValue(decision.Phase), NormalizedEventKey: NormalizeSignature(decision.NormalizedEventKey),
		NormalizedStackKey: NormalizeSignature(decision.NormalizedStackKey), RegistryHash: strings.TrimSpace(decision.Token.RegistryHash),
	}
}

func normalizeClosedValue(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "unknown"
	}
	return v
}

func ClusterID(fp types.TraceCauseFingerprintV1) (string, error) {
	b, err := json.Marshal(fp)
	if err != nil {
		return "", fmt.Errorf("marshal fingerprint: %w", err)
	}
	sum := sha256.Sum256(b)
	return "rc-" + hex.EncodeToString(sum[:]), nil
}

// CanonicalLabel is intentionally short and independent of ClusterID.
func CanonicalLabel(fp types.TraceCauseFingerprintV1) string {
	parts := []string{fp.Token}
	if fp.SubjectRole != "" && fp.SubjectRole != "unknown" {
		parts = append(parts, fp.SubjectRole)
	}
	if fp.UpstreamRole != "" && fp.UpstreamRole != "unknown" {
		parts = append(parts, "→"+fp.UpstreamRole)
	}
	if fp.Phase != "" && fp.Phase != "unknown" {
		parts = append(parts, "@"+fp.Phase)
	}
	return strings.Join(parts, " ")
}

type Input struct {
	UnitID  string
	Finding types.TraceFindingV1
}

// Exact builds the deterministic baseline. Each resolved finding enters one
// primary cluster; unresolved findings remain explicit and never disappear.
func Exact(batchID string, inputs []Input, failures []types.TraceBatchFailure) types.TraceRootCauseClusterSetV1 {
	out := types.TraceRootCauseClusterSetV1{SchemaVersion: types.TraceRootCauseClusterSchemaVersion, BatchID: batchID,
		FingerprintVersion: FingerprintVersion, InputUnitCount: len(inputs) + len(failures), Failures: append([]types.TraceBatchFailure(nil), failures...), FailedCount: len(failures)}
	groups := map[string]*types.TraceRootCauseCluster{}
	for _, in := range inputs {
		if err := tracefinding.ValidateStored(&in.Finding); err != nil {
			out.Failures = append(out.Failures, types.TraceBatchFailure{UnitID: in.UnitID, Code: "finding_schema_invalid", Detail: err.Error()})
			out.FailedCount++
			continue
		}
		out.SuccessfulCount++
		if in.Finding.PrimaryCause == nil {
			reason := "根因证据不足"
			if in.Finding.Unresolved != nil && strings.TrimSpace(in.Finding.Unresolved.Reason) != "" {
				reason = in.Finding.Unresolved.Reason
			}
			out.Unresolved = append(out.Unresolved, types.TraceUnresolvedMember{UnitID: in.UnitID, FindingID: in.Finding.FindingID, Reason: reason})
			continue
		}
		fp := Fingerprint(*in.Finding.PrimaryCause)
		id, err := ClusterID(fp)
		if err != nil {
			out.Failures = append(out.Failures, types.TraceBatchFailure{UnitID: in.UnitID, Code: "cluster_fingerprint_failed", Detail: err.Error()})
			out.FailedCount++
			out.SuccessfulCount--
			continue
		}
		cluster := groups[id]
		if cluster == nil {
			cluster = &types.TraceRootCauseCluster{ClusterID: id, Level: "L2", Fingerprint: fp, CanonicalLabel: CanonicalLabel(fp), MergeBasis: []string{"exact_fingerprint"}, AmbiguityStatus: "exact"}
			groups[id] = cluster
		}
		cluster.PrimaryMembers = append(cluster.PrimaryMembers, types.TraceClusterMember{UnitID: in.UnitID, FindingID: in.Finding.FindingID, AnalysisKey: in.Finding.AnalysisKey})
		cluster.PrimaryCount++
		addMagnitude(cluster, in.Finding.PrimaryCause.Magnitude)
		for _, contributor := range in.Finding.Contributors {
			contributorFP := Fingerprint(contributor)
			contributorID, err := ClusterID(contributorFP)
			if err != nil {
				continue
			}
			contributorCluster := groups[contributorID]
			if contributorCluster == nil {
				contributorCluster = &types.TraceRootCauseCluster{ClusterID: contributorID, Level: "L2", Fingerprint: contributorFP, CanonicalLabel: CanonicalLabel(contributorFP), MergeBasis: []string{"exact_fingerprint"}, AmbiguityStatus: "exact"}
				groups[contributorID] = contributorCluster
			}
			contributorCluster.ContributorMembers = append(contributorCluster.ContributorMembers, types.TraceClusterMember{UnitID: in.UnitID, FindingID: in.Finding.FindingID, AnalysisKey: in.Finding.AnalysisKey})
		}
	}
	out.UnresolvedCount = len(out.Unresolved)
	out.ResolvedCount = out.SuccessfulCount - out.UnresolvedCount
	for _, c := range groups {
		sort.Slice(c.PrimaryMembers, func(i, j int) bool { return c.PrimaryMembers[i].UnitID < c.PrimaryMembers[j].UnitID })
		sort.Slice(c.ContributorMembers, func(i, j int) bool { return c.ContributorMembers[i].UnitID < c.ContributorMembers[j].UnitID })
		if len(c.PrimaryMembers) > 0 {
			member := c.PrimaryMembers[0]
			c.Representatives = []types.TraceFindingRef{{FindingID: member.FindingID, AnalysisKey: member.AnalysisKey}}
		} else if len(c.ContributorMembers) > 0 {
			member := c.ContributorMembers[0]
			c.Representatives = []types.TraceFindingRef{{FindingID: member.FindingID, AnalysisKey: member.AnalysisKey}}
		}
		c.Singleton = c.PrimaryCount == 1
		if out.SuccessfulCount > 0 {
			c.ShareOfAllSuccessful = float64(c.PrimaryCount) / float64(out.SuccessfulCount)
		}
		if out.ResolvedCount > 0 {
			c.ShareOfResolved = float64(c.PrimaryCount) / float64(out.ResolvedCount)
		}
		sort.Slice(c.MetricBuckets, func(i, j int) bool { return c.MetricBuckets[i].Key < c.MetricBuckets[j].Key })
		for i := range c.MetricBuckets {
			sort.Float64s(c.MetricBuckets[i].Values)
		}
		out.Clusters = append(out.Clusters, *c)
	}
	sort.Slice(out.Clusters, func(i, j int) bool { return out.Clusters[i].ClusterID < out.Clusters[j].ClusterID })
	sort.Slice(out.Unresolved, func(i, j int) bool { return out.Unresolved[i].UnitID < out.Unresolved[j].UnitID })
	out.Invariants = Validate(out)
	return out
}

func addMagnitude(c *types.TraceRootCauseCluster, m *types.TypedMagnitude) {
	if m == nil {
		return
	}
	key := strings.Join([]string{m.Unit, m.Additivity, m.Caliber}, "|")
	for i := range c.MetricBuckets {
		if c.MetricBuckets[i].Key == key {
			c.MetricBuckets[i].Values = append(c.MetricBuckets[i].Values, m.Value)
			return
		}
	}
	c.MetricBuckets = append(c.MetricBuckets, types.TraceClusterMetricBucket{Key: key, Unit: m.Unit, Additivity: m.Additivity, Caliber: m.Caliber, Values: []float64{m.Value}})
}

func Validate(set types.TraceRootCauseClusterSetV1) types.ClusterInvariantReport {
	r := types.ClusterInvariantReport{Valid: true}
	seen := map[string]bool{}
	primary := 0
	for _, c := range set.Clusters {
		if c.PrimaryCount != len(c.PrimaryMembers) {
			r.Errors = append(r.Errors, "cluster "+c.ClusterID+": primary_count mismatch")
		}
		primary += len(c.PrimaryMembers)
		for _, m := range c.PrimaryMembers {
			key := m.UnitID + "\x00" + m.FindingID
			if seen[key] {
				r.Errors = append(r.Errors, "duplicate primary member: "+m.UnitID)
			}
			seen[key] = true
		}
	}
	if set.SuccessfulCount != primary+set.UnresolvedCount {
		r.Errors = append(r.Errors, "successful_count conservation failed")
	}
	if set.UnresolvedCount != len(set.Unresolved) {
		r.Errors = append(r.Errors, "unresolved_count mismatch")
	}
	if set.FailedCount != len(set.Failures) {
		r.Errors = append(r.Errors, "failed_count mismatch")
	}
	r.Valid = len(r.Errors) == 0
	return r
}
