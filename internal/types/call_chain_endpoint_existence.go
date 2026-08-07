package types

import (
	"path"
	"strconv"
	"strings"
)

// CallChainEndpointExistenceProof describes which precise current-source
// carrier proves an endpoint identity. Definitions prove existence only;
// call-edge participation additionally proves that some incident topology was
// emitted, but never by itself establishes the requested source->sink path.
type CallChainEndpointExistenceProof string

const (
	CallChainEndpointExistenceUnproven          CallChainEndpointExistenceProof = "unproven"
	CallChainEndpointExistenceDefinitionOnly    CallChainEndpointExistenceProof = "definition_only"
	CallChainEndpointExistenceCallEdge          CallChainEndpointExistenceProof = "call_edge"
	CallChainEndpointExistenceDefinitionAndEdge CallChainEndpointExistenceProof = "definition_and_call_edge"
	CallChainEndpointExistenceAmbiguous         CallChainEndpointExistenceProof = "ambiguous"
)

// CallChainEndpointExistenceAnalysis records whether both requested endpoints
// exist in citable current source. It is intentionally independent from graph
// reachability: a definition can prove that a leaf endpoint exists, but it can
// never mint a call edge or a path.
type CallChainEndpointExistenceAnalysis struct {
	StartProven    bool
	EndProven      bool
	StartAmbiguous bool
	EndAmbiguous   bool
	StartProof     CallChainEndpointExistenceProof
	EndProof       CallChainEndpointExistenceProof
}

// AnalyzeCallChainEndpointExistence consumes only grounded call/definition
// carriers. It never reads request prose, waiver rationale, source order, or a
// prefix sibling such as RunWith for Run.
func AnalyzeCallChainEndpointExistence(evidence []EvidenceItem, startHint, endHint string) CallChainEndpointExistenceAnalysis {
	identities := callChainEndpointEvidenceProofs(evidence)
	startProof, startAmbiguous := resolveCallChainEndpointExistence(identities, startHint)
	endProof, endAmbiguous := resolveCallChainEndpointExistence(identities, endHint)
	if startProof == CallChainEndpointExistenceUnproven && sharedCallChainQualifiedOwner(sharedCallChainIdentity(startHint)) != "" {
		startProof, startAmbiguous = resolveCallChainScopedBareDefinitionExistence(evidence, startHint)
	}
	if endProof == CallChainEndpointExistenceUnproven && sharedCallChainQualifiedOwner(sharedCallChainIdentity(endHint)) != "" {
		endProof, endAmbiguous = resolveCallChainScopedBareDefinitionExistence(evidence, endHint)
	}
	return CallChainEndpointExistenceAnalysis{
		StartProven:    startProof != CallChainEndpointExistenceUnproven && startProof != CallChainEndpointExistenceAmbiguous,
		EndProven:      endProof != CallChainEndpointExistenceUnproven && endProof != CallChainEndpointExistenceAmbiguous,
		StartAmbiguous: startAmbiguous, EndAmbiguous: endAmbiguous,
		StartProof: startProof, EndProof: endProof,
	}
}

// resolveCallChainScopedBareDefinitionExistence recovers one narrow identity
// loss at the evidence boundary: parsers and model emissions sometimes retain
// only a declaration's local operation (`Run`) while the typed endpoint keeps
// its package/module/type owner (`gate.Run`). A grounded definition may prove
// that endpoint only when (a) the operation tail is exact, (b) the requested
// owner matches the defining file stem or its immediate package directory,
// and (c) exactly one current-source location survives. This never creates a
// call edge or reachability. Multiple scoped definitions remain ambiguous.
func resolveCallChainScopedBareDefinitionExistence(evidence []EvidenceItem, endpoint string) (CallChainEndpointExistenceProof, bool) {
	want := strings.ToLower(sharedCallChainIdentity(endpoint))
	owner := strings.ToLower(sharedCallChainQualifiedOwner(want))
	operation := strings.ToLower(sharedCallChainQualifiedOperation(want))
	if owner == "" || operation == "" {
		return CallChainEndpointExistenceUnproven, false
	}
	ownerTail := strings.ToLower(sharedCallChainQualifiedOperation(owner))
	if ownerTail == "" {
		ownerTail = owner
	}
	matches := map[string]bool{}
	for _, item := range evidence {
		if !item.IsCitable() || RuntimeArtifactPathKind(item.Source) != "" ||
			!HasCodeOrConfigPathSuffix(strings.ToLower(item.Source)) ||
			ClaimFormOf(item) != ClaimDefinitionFact {
			continue
		}
		identity := strings.TrimSpace(item.Subject)
		if identity == "" {
			identity = strings.TrimSpace(item.AnchorSymbol)
		}
		identity = strings.ToLower(sharedCallChainIdentity(identity))
		if identity == "" || sharedCallChainQualifiedOwner(identity) != "" ||
			strings.ToLower(sharedCallChainQualifiedOperation(identity)) != operation {
			continue
		}
		if !callChainDefinitionOwnerMetadataMatches(item, want, operation) &&
			!callChainDefinitionSourceMatchesOwner(item.Source, ownerTail) {
			continue
		}
		location := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(item.Source), `\`, `/`)) + ":" + strconv.Itoa(item.LineStart)
		matches[location] = true
	}
	switch len(matches) {
	case 1:
		return CallChainEndpointExistenceDefinitionOnly, false
	case 0:
		return CallChainEndpointExistenceUnproven, false
	default:
		return CallChainEndpointExistenceAmbiguous, true
	}
}

func callChainDefinitionOwnerMetadataMatches(item EvidenceItem, endpoint, operation string) bool {
	owner := strings.ToLower(sharedCallChainIdentity(item.OwnerSymbol))
	if owner == "" || owner == operation {
		return false
	}
	if owner == endpoint {
		return true
	}
	return strings.ToLower(sharedCallChainIdentity(owner+"."+operation)) == endpoint
}

func callChainDefinitionSourceMatchesOwner(source, owner string) bool {
	source = strings.Trim(strings.ReplaceAll(strings.TrimSpace(source), `\`, `/`), "/")
	owner = strings.ToLower(strings.TrimSpace(owner))
	if source == "" || owner == "" {
		return false
	}
	base := path.Base(source)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	parent := path.Base(path.Dir(source))
	return strings.EqualFold(stem, owner) || strings.EqualFold(parent, owner)
}

type callChainEndpointExistenceProofBits uint8

const (
	callChainEndpointProofDefinition callChainEndpointExistenceProofBits = 1 << iota
	callChainEndpointProofCallEdge
)

func callChainEndpointEvidenceProofs(evidence []EvidenceItem) map[string]callChainEndpointExistenceProofBits {
	out := map[string]callChainEndpointExistenceProofBits{}
	add := func(raw string, proof callChainEndpointExistenceProofBits) {
		identity := strings.ToLower(sharedCallChainIdentity(raw))
		if identity == "" {
			return
		}
		out[identity] |= proof
	}
	for _, item := range evidence {
		if !item.IsCitable() || RuntimeArtifactPathKind(item.Source) != "" ||
			!HasCodeOrConfigPathSuffix(strings.ToLower(item.Source)) {
			continue
		}
		switch ClaimFormOf(item) {
		case ClaimCallEdge:
			add(item.Subject, callChainEndpointProofCallEdge)
			if strings.TrimSpace(item.Object) != "" {
				add(item.Object, callChainEndpointProofCallEdge)
			} else {
				add(item.AnchorSymbol, callChainEndpointProofCallEdge)
			}
		case ClaimDefinitionFact:
			// A grounded definition's visible anchor is an independent typed
			// identity carrier. Model emissions and language parsers may put the
			// enclosing package/type in Subject (for example subject="gate",
			// anchor_symbol="gate.Run"). Ignoring the anchor whenever Subject is
			// non-empty makes the grounder accept and display the exact endpoint
			// while this authority layer calls it absent. Preserve both carriers;
			// exact matching below still rejects prefix siblings such as RunWith
			// for Run and keeps ambiguous short names fail-closed.
			add(item.AnchorSymbol, callChainEndpointProofDefinition)
			if strings.TrimSpace(item.Subject) != "" {
				add(item.Subject, callChainEndpointProofDefinition)
			} else if owner, anchor := strings.TrimSpace(item.OwnerSymbol), strings.TrimSpace(item.AnchorSymbol); owner != "" && anchor != "" {
				add(owner+"."+anchor, callChainEndpointProofDefinition)
			} else {
				add(item.AnchorSymbol, callChainEndpointProofDefinition)
			}
		}
	}
	return out
}

func resolveCallChainEndpointExistence(identities map[string]callChainEndpointExistenceProofBits, endpoint string) (CallChainEndpointExistenceProof, bool) {
	want := strings.ToLower(sharedCallChainIdentity(endpoint))
	if want == "" {
		return CallChainEndpointExistenceUnproven, false
	}
	if sharedCallChainQualifiedOwner(want) != "" {
		if proof := identities[want]; proof != 0 {
			return callChainEndpointExistenceProofFromBits(proof), false
		}
		return CallChainEndpointExistenceUnproven, false
	}
	matches := map[string]callChainEndpointExistenceProofBits{}
	for identity, proof := range identities {
		if identity == want || strings.EqualFold(sharedCallChainQualifiedOperation(identity), want) {
			matches[identity] = proof
		}
	}
	if len(matches) > 1 {
		return CallChainEndpointExistenceAmbiguous, true
	}
	for _, proof := range matches {
		return callChainEndpointExistenceProofFromBits(proof), false
	}
	return CallChainEndpointExistenceUnproven, false
}

func callChainEndpointExistenceProofFromBits(bits callChainEndpointExistenceProofBits) CallChainEndpointExistenceProof {
	switch bits {
	case callChainEndpointProofDefinition:
		return CallChainEndpointExistenceDefinitionOnly
	case callChainEndpointProofCallEdge:
		return CallChainEndpointExistenceCallEdge
	case callChainEndpointProofDefinition | callChainEndpointProofCallEdge:
		return CallChainEndpointExistenceDefinitionAndEdge
	default:
		return CallChainEndpointExistenceUnproven
	}
}
