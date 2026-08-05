package types

import "strings"

// CallChainEndpointExistenceAnalysis records whether both requested endpoints
// exist in citable current source. It is intentionally independent from graph
// reachability: a definition can prove that a leaf endpoint exists, but it can
// never mint a call edge or a path.
type CallChainEndpointExistenceAnalysis struct {
	StartProven    bool
	EndProven      bool
	StartAmbiguous bool
	EndAmbiguous   bool
}

// AnalyzeCallChainEndpointExistence consumes only grounded call/definition
// carriers. It never reads request prose, waiver rationale, source order, or a
// prefix sibling such as RunWith for Run.
func AnalyzeCallChainEndpointExistence(evidence []EvidenceItem, startHint, endHint string) CallChainEndpointExistenceAnalysis {
	identities := callChainEndpointEvidenceIdentities(evidence)
	start, startAmbiguous := resolveCallChainEndpointExistence(identities, startHint)
	end, endAmbiguous := resolveCallChainEndpointExistence(identities, endHint)
	return CallChainEndpointExistenceAnalysis{
		StartProven: start, EndProven: end,
		StartAmbiguous: startAmbiguous, EndAmbiguous: endAmbiguous,
	}
}

func callChainEndpointEvidenceIdentities(evidence []EvidenceItem) []string {
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		identity := strings.ToLower(sharedCallChainIdentity(raw))
		if identity == "" || seen[identity] {
			return
		}
		seen[identity] = true
		out = append(out, identity)
	}
	for _, item := range evidence {
		if !item.IsCitable() || RuntimeArtifactPathKind(item.Source) != "" ||
			!HasCodeOrConfigPathSuffix(strings.ToLower(item.Source)) {
			continue
		}
		switch ClaimFormOf(item) {
		case ClaimCallEdge:
			add(item.Subject)
			if strings.TrimSpace(item.Object) != "" {
				add(item.Object)
			} else {
				add(item.AnchorSymbol)
			}
		case ClaimDefinitionFact:
			if strings.TrimSpace(item.Subject) != "" {
				add(item.Subject)
			} else if owner, anchor := strings.TrimSpace(item.OwnerSymbol), strings.TrimSpace(item.AnchorSymbol); owner != "" && anchor != "" {
				add(owner + "." + anchor)
			} else {
				add(item.AnchorSymbol)
			}
		}
	}
	return out
}

func resolveCallChainEndpointExistence(identities []string, endpoint string) (bool, bool) {
	want := strings.ToLower(sharedCallChainIdentity(endpoint))
	if want == "" {
		return false, false
	}
	if sharedCallChainQualifiedOwner(want) != "" {
		for _, identity := range identities {
			if identity == want {
				return true, false
			}
		}
		return false, false
	}
	matches := map[string]bool{}
	for _, identity := range identities {
		if identity == want || strings.EqualFold(sharedCallChainQualifiedOperation(identity), want) {
			matches[identity] = true
		}
	}
	return len(matches) == 1, len(matches) > 1
}
