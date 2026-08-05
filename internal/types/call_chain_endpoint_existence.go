package types

import "strings"

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
	return CallChainEndpointExistenceAnalysis{
		StartProven:    startProof != CallChainEndpointExistenceUnproven && startProof != CallChainEndpointExistenceAmbiguous,
		EndProven:      endProof != CallChainEndpointExistenceUnproven && endProof != CallChainEndpointExistenceAmbiguous,
		StartAmbiguous: startAmbiguous, EndAmbiguous: endAmbiguous,
		StartProof: startProof, EndProof: endProof,
	}
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
