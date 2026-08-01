package types

import (
	"sort"
	"strings"
)

type RuntimeArtifactPairRelationStatus string

const (
	RuntimeArtifactPairRelationUnproven RuntimeArtifactPairRelationStatus = "unproven"
)

type RuntimeArtifactRelationEndpoint struct {
	ArtifactID          string
	Path                string
	ArtifactKind        string
	TimeDomain          string
	CanonicalTimeDomain string
	ClockAlignment      string
	ClockCalibrated     bool
}

type RuntimeArtifactPairRelation struct {
	Left                   RuntimeArtifactRelationEndpoint
	Right                  RuntimeArtifactRelationEndpoint
	SharedClockOrigin      RuntimeArtifactPairRelationStatus
	DirectTimeAlignment    RuntimeArtifactPairRelationStatus
	SharedDevice           RuntimeArtifactPairRelationStatus
	SharedCaptureSession   RuntimeArtifactPairRelationStatus
	SameTimeDomainLabel    bool
	SameCanonicalDomain    bool
	LocalIdentityOnly      bool
	MissingRelationAnchors []string
}

type RuntimeArtifactPairRelationAuthority struct {
	Active    bool
	Artifacts []RuntimeArtifactRelationEndpoint
	Pairs     []RuntimeArtifactPairRelation
}

// BuildRuntimeArtifactPairRelationAuthority derives only cross-artifact
// relation boundaries. Endpoint-local time-domain/alignment fields never prove
// a relation between two independently identified runtime artifacts.
func BuildRuntimeArtifactPairRelationAuthority(ledger ObservationLedger) RuntimeArtifactPairRelationAuthority {
	byID := map[string]*runtimeArtifactRelationEndpointAccumulator{}
	for _, record := range ledger.Records {
		if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			!RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
			continue
		}
		key, id := runtimeArtifactRelationIdentity(record.SourceRef)
		if key == "" || id == "" {
			continue
		}
		acc := byID[key]
		if acc == nil {
			acc = &runtimeArtifactRelationEndpointAccumulator{id: id}
			byID[key] = acc
		}
		acc.add(record.SourceRef)
	}
	if len(byID) < 2 {
		return RuntimeArtifactPairRelationAuthority{}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	artifacts := make([]RuntimeArtifactRelationEndpoint, 0, len(ids))
	for _, id := range ids {
		artifacts = append(artifacts, byID[id].endpoint())
	}
	pairs := make([]RuntimeArtifactPairRelation, 0, len(artifacts)*(len(artifacts)-1)/2)
	for i := range artifacts {
		for j := i + 1; j < len(artifacts); j++ {
			left, right := artifacts[i], artifacts[j]
			pairs = append(pairs, RuntimeArtifactPairRelation{
				Left:                 left,
				Right:                right,
				SharedClockOrigin:    RuntimeArtifactPairRelationUnproven,
				DirectTimeAlignment:  RuntimeArtifactPairRelationUnproven,
				SharedDevice:         RuntimeArtifactPairRelationUnproven,
				SharedCaptureSession: RuntimeArtifactPairRelationUnproven,
				SameTimeDomainLabel: runtimeArtifactRelationSameSingleValue(
					left.TimeDomain, right.TimeDomain,
				),
				SameCanonicalDomain: runtimeArtifactRelationSameSingleValue(
					left.CanonicalTimeDomain, right.CanonicalTimeDomain,
				),
				LocalIdentityOnly: strings.EqualFold(left.ClockAlignment, "identity") &&
					strings.EqualFold(right.ClockAlignment, "identity"),
				MissingRelationAnchors: []string{
					"shared_clock_calibration_anchor",
					"shared_device_anchor",
					"shared_capture_session_anchor",
				},
			})
		}
	}
	return RuntimeArtifactPairRelationAuthority{Active: true, Artifacts: artifacts, Pairs: pairs}
}

func runtimeArtifactRelationIdentity(ref ObservationSourceRef) (key, displayID string) {
	id := strings.TrimSpace(ref.ArtifactID)
	// The runtime attachment path identifies the immutable turn artifact.
	// Different query/supplement producers may assign different local IDs to
	// that same blob; grouping by ID first would mint a false cross-artifact
	// pair for one physical input. Conversely, one generic ID can be reused
	// across two paths, so a present canonical path must remain primary.
	path := runtimeArtifactIdentityPathKey(RuntimeArtifactCaptureIdentityPath(ref))
	if path != "" {
		return "artifact_path\x00" + path, "runtime_artifact:" + RuntimeArtifactHashString(
			path,
		)
	}
	if id != "" && !strings.EqualFold(id, "trace_query") {
		return "artifact_id\x00" + id, id
	}
	return "", ""
}

type runtimeArtifactRelationEndpointAccumulator struct {
	id                  string
	paths               map[string]bool
	kinds               map[string]bool
	timeDomains         map[string]bool
	canonicalDomains    map[string]bool
	alignments          map[string]bool
	clockCalibratedSeen bool
}

func (acc *runtimeArtifactRelationEndpointAccumulator) add(ref ObservationSourceRef) {
	runtimeArtifactRelationAddValue(&acc.paths, runtimeArtifactIdentityPathKey(RuntimeArtifactCaptureIdentityPath(ref)))
	runtimeArtifactRelationAddValue(&acc.kinds, ref.ArtifactKind)
	runtimeArtifactRelationAddValue(&acc.timeDomains, ref.TimeDomain)
	runtimeArtifactRelationAddValue(&acc.canonicalDomains, ref.CanonicalTimeDomain)
	runtimeArtifactRelationAddValue(&acc.alignments, ref.ClockAlignment)
	acc.clockCalibratedSeen = acc.clockCalibratedSeen || ref.ClockCalibrated
}

func (acc runtimeArtifactRelationEndpointAccumulator) endpoint() RuntimeArtifactRelationEndpoint {
	return RuntimeArtifactRelationEndpoint{
		ArtifactID:          acc.id,
		Path:                runtimeArtifactRelationSingleValue(acc.paths),
		ArtifactKind:        runtimeArtifactRelationSingleValue(acc.kinds),
		TimeDomain:          runtimeArtifactRelationSingleValue(acc.timeDomains),
		CanonicalTimeDomain: runtimeArtifactRelationSingleValue(acc.canonicalDomains),
		ClockAlignment:      runtimeArtifactRelationSingleValue(acc.alignments),
		ClockCalibrated:     acc.clockCalibratedSeen,
	}
}

func runtimeArtifactRelationAddValue(dst *map[string]bool, raw string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return
	}
	if *dst == nil {
		*dst = map[string]bool{}
	}
	(*dst)[value] = true
}

func runtimeArtifactRelationSingleValue(values map[string]bool) string {
	if len(values) != 1 {
		if len(values) > 1 {
			return "multiple"
		}
		return ""
	}
	for value := range values {
		return value
	}
	return ""
}

func runtimeArtifactRelationSameSingleValue(left, right string) bool {
	return left != "" && right != "" && left != "multiple" && right != "multiple" &&
		strings.EqualFold(left, right)
}
