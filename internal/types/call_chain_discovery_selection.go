package types

import "strings"

// CallChainDiscoverySelectionEvidence returns the citable typed facts that can
// select a concrete destination for a discover-sink call-chain request.
//
// Registration is already a complete binding edge and is accepted directly.
// Assignment/initializer and return facts must be connected to a citable call
// endpoint (or to a registration endpoint) in the same evidence pool. This
// prevents an unrelated return elsewhere in the repository from closing the
// destination-discovery contract. The matcher consumes only typed evidence
// fields; it never scans request prose, summaries, answer text, language names,
// or repository-specific keywords.
func CallChainDiscoverySelectionEvidence(evidence []EvidenceItem) []EvidenceItem {
	if len(evidence) == 0 {
		return nil
	}

	var connectionEndpoints []string
	var registrations []EvidenceItem
	for _, item := range evidence {
		if !item.IsCitable() {
			continue
		}
		switch ClaimFormOf(item) {
		case ClaimCallEdge:
			connectionEndpoints = appendCallChainDiscoveryEndpoint(connectionEndpoints, item.Subject)
			connectionEndpoints = appendCallChainDiscoveryEndpoint(connectionEndpoints, item.Object)
			connectionEndpoints = appendCallChainDiscoveryEndpoint(connectionEndpoints, item.AnchorSymbol)
		case ClaimRegistrationEdge:
			if strings.TrimSpace(item.Subject) == "" || strings.TrimSpace(item.Object) == "" {
				continue
			}
			registrations = append(registrations, item)
			connectionEndpoints = appendCallChainDiscoveryEndpoint(connectionEndpoints, item.Subject)
			connectionEndpoints = appendCallChainDiscoveryEndpoint(connectionEndpoints, item.Object)
		}
	}

	out := append([]EvidenceItem(nil), registrations...)
	for _, item := range evidence {
		if !item.IsCitable() || strings.TrimSpace(item.Subject) == "" || strings.TrimSpace(item.Object) == "" {
			continue
		}
		form := ClaimFormOf(item)
		if form != ClaimAssignmentFact && form != ClaimReturnFact {
			continue
		}
		if callChainDiscoveryEndpointConnected(item.Subject, connectionEndpoints) {
			out = append(out, item)
		}
	}
	return out
}

// HasCallChainDiscoverySelectionEvidence is the boolean closure predicate used
// by explorer readiness and the pre-complete safeguard. Keeping it here makes
// both gates consume one typed authority definition.
func HasCallChainDiscoverySelectionEvidence(evidence []EvidenceItem) bool {
	return len(CallChainDiscoverySelectionEvidence(evidence)) > 0
}

func appendCallChainDiscoveryEndpoint(dst []string, raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return dst
	}
	return append(dst, raw)
}

func callChainDiscoveryEndpointConnected(subject string, endpoints []string) bool {
	for _, endpoint := range endpoints {
		if CallChainEndpointCompatible(subject, endpoint) || CallChainEndpointCompatible(endpoint, subject) {
			return true
		}
		if receiver := callChainDiscoveryReceiver(endpoint); receiver != "" &&
			(CallChainEndpointCompatible(subject, receiver) || CallChainEndpointCompatible(receiver, subject)) {
			return true
		}
	}
	return false
}

// callChainDiscoveryReceiver peels exactly one invocation/member operation from
// a typed endpoint. It covers the stable separator family used by the supported
// languages (dot, C/C++ pointer member, and namespace/associated item) without
// guessing from natural-language words or accepting arbitrary prefixes.
func callChainDiscoveryReceiver(endpoint string) string {
	normalized := strings.TrimSpace(endpoint)
	normalized = strings.ReplaceAll(normalized, "->", ".")
	normalized = strings.ReplaceAll(normalized, "::", ".")
	normalized = strings.ReplaceAll(normalized, "#", ".")
	idx := strings.LastIndex(normalized, ".")
	if idx <= 0 {
		return ""
	}
	receiver := strings.TrimSpace(normalized[:idx])
	receiver = strings.TrimSuffix(strings.TrimSuffix(receiver, "?"), "!")
	if !IsCodeIdentitySurface(receiver) {
		return ""
	}
	return receiver
}
