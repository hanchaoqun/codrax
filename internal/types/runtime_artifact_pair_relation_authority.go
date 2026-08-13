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
	carrierOwners := buildRuntimeArtifactDerivedCarrierOwners(ledger)
	byID := map[string]*runtimeArtifactRelationEndpointAccumulator{}
	for _, record := range ledger.Records {
		if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			!RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
			continue
		}
		ref := record.SourceRef
		key, id := runtimeArtifactRelationIdentity(ref)
		if owner, ok := carrierOwners.uniqueOwnerForPath(ref.Path); ok {
			key, id = owner.key, owner.id
			// Path remains the exact carrier address for citation/read purposes.
			// Pair authority, however, must group a producer-owned payload with
			// the physical capture that produced it. Feeding the inherited
			// capture identity through the existing accumulator chokepoint also
			// preserves metadata carried by the derived query without exposing
			// the private payload as an independent endpoint.
			ref.CaptureIdentityPath = owner.capturePath
		}
		if key == "" || id == "" {
			continue
		}
		acc := byID[key]
		if acc == nil {
			acc = &runtimeArtifactRelationEndpointAccumulator{id: id}
			byID[key] = acc
		}
		acc.add(ref)
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

type runtimeArtifactDerivedCarrierOwner struct {
	key         string
	id          string
	capturePath string
}

type runtimeArtifactDerivedCarrierOwnerSet map[string]runtimeArtifactDerivedCarrierOwner

type runtimeArtifactDerivedCarrierOwners map[string]runtimeArtifactDerivedCarrierOwnerSet

// runtimeArtifactDerivedCarrierOwners records only exact, typed producer to
// carrier relations. It deliberately does not infer ownership from a filename,
// extension, directory, artifact ID, or model prose. A carrier referenced by
// multiple physical captures remains ambiguous and therefore cannot collapse
// either endpoint.
func buildRuntimeArtifactDerivedCarrierOwners(ledger ObservationLedger) runtimeArtifactDerivedCarrierOwners {
	parents := map[string][]ObservationSourceRef{}
	for _, record := range ledger.Records {
		if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			!RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
			continue
		}
		for _, carrier := range runtimeArtifactDerivedCarrierRefs(record.SourceRef) {
			carrierKey := runtimeArtifactIdentityPathKey(carrier)
			if carrierKey == "" || carrierKey == runtimeArtifactIdentityPathKey(RuntimeArtifactCaptureIdentityPath(record.SourceRef)) {
				continue
			}
			parents[carrierKey] = append(parents[carrierKey], record.SourceRef)
		}
	}
	owners := runtimeArtifactDerivedCarrierOwners{}
	for _, record := range ledger.Records {
		if record.Origin != AnswerEvidenceOriginRuntimeArtifact ||
			!RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
			continue
		}
		owner, ok := runtimeArtifactDerivedCarrierRoot(record.SourceRef, parents, map[string]bool{})
		if !ok {
			continue
		}
		for _, carrier := range runtimeArtifactDerivedCarrierRefs(record.SourceRef) {
			carrierKey := runtimeArtifactIdentityPathKey(carrier)
			if carrierKey == "" || carrierKey == runtimeArtifactIdentityPathKey(owner.capturePath) {
				continue
			}
			if owners[carrierKey] == nil {
				owners[carrierKey] = runtimeArtifactDerivedCarrierOwnerSet{}
			}
			owners[carrierKey][owner.key] = owner
		}
	}
	return owners
}

func runtimeArtifactDerivedCarrierRefs(ref ObservationSourceRef) []string {
	return []string{ref.PayloadRef, ref.RawRef, ref.RowSetRef, ref.PageRef}
}

func runtimeArtifactDerivedCarrierRoot(
	ref ObservationSourceRef,
	parents map[string][]ObservationSourceRef,
	visiting map[string]bool,
) (runtimeArtifactDerivedCarrierOwner, bool) {
	pathKey := runtimeArtifactIdentityPathKey(ref.Path)
	if pathKey != "" && len(parents[pathKey]) > 0 {
		if visiting[pathKey] {
			return runtimeArtifactDerivedCarrierOwner{}, false
		}
		visiting[pathKey] = true
		roots := runtimeArtifactDerivedCarrierOwnerSet{}
		for _, parent := range parents[pathKey] {
			root, ok := runtimeArtifactDerivedCarrierRoot(parent, parents, visiting)
			if !ok {
				delete(visiting, pathKey)
				return runtimeArtifactDerivedCarrierOwner{}, false
			}
			roots[root.key] = root
		}
		delete(visiting, pathKey)
		if len(roots) != 1 {
			return runtimeArtifactDerivedCarrierOwner{}, false
		}
		for _, root := range roots {
			return root, true
		}
	}
	key, id := runtimeArtifactRelationIdentity(ref)
	capturePath := RuntimeArtifactCaptureIdentityPath(ref)
	if key == "" || id == "" || strings.TrimSpace(capturePath) == "" {
		return runtimeArtifactDerivedCarrierOwner{}, false
	}
	return runtimeArtifactDerivedCarrierOwner{key: key, id: id, capturePath: capturePath}, true
}

func (owners runtimeArtifactDerivedCarrierOwners) uniqueOwnerForPath(path string) (runtimeArtifactDerivedCarrierOwner, bool) {
	set := owners[runtimeArtifactIdentityPathKey(path)]
	if len(set) != 1 {
		return runtimeArtifactDerivedCarrierOwner{}, false
	}
	for _, owner := range set {
		return owner, true
	}
	return runtimeArtifactDerivedCarrierOwner{}, false
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
