package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const answerDocRuntimeArtifactPairPromptLimit = 8

func renderAnswerDocRuntimeArtifactPairRelationAuthority(ledger types.ObservationLedger) string {
	authority := types.BuildRuntimeArtifactPairRelationAuthority(ledger)
	if !authority.Active || len(authority.Pairs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Typed Cross-Artifact Relation Authority\n\n")
	b.WriteString("- This authority is derived only from accepted deterministic runtime-artifact identities and endpoint-local clock metadata. It is an answer-writing boundary, not a reason to reopen exploration.\n")
	b.WriteString("- Endpoint-local `alignment=identity` means that endpoint needed no local transform. Matching time-domain labels or canonical-domain labels do not prove a shared clock origin between independent artifacts.\n")
	b.WriteString("- Without a typed shared anchor, do not claim that either artifact pair came from the same or different device/session, shares a clock origin, or can be directly time-aligned. A subtraction of local timestamps is only a numeric offset, not calibrated capture separation.\n")
	for i, pair := range authority.Pairs {
		if i >= answerDocRuntimeArtifactPairPromptLimit {
			fmt.Fprintf(&b, "- omitted_pairs=%d; every omitted pair has the same fail-closed relation boundary unless a typed shared anchor is added\n",
				len(authority.Pairs)-i)
			break
		}
		fmt.Fprintf(&b,
			"- pair[%d] left=`%s`; right=`%s`; shared_clock_origin=`%s`; direct_time_alignment=`%s`; shared_device=`%s`; shared_capture_session=`%s`; same_time_domain_label=%t; same_canonical_domain_label=%t; local_identity_only=%t; missing_anchors=`%s`\n",
			i+1,
			runtimeArtifactRelationEndpointLabel(pair.Left),
			runtimeArtifactRelationEndpointLabel(pair.Right),
			pair.SharedClockOrigin,
			pair.DirectTimeAlignment,
			pair.SharedDevice,
			pair.SharedCaptureSession,
			pair.SameTimeDomainLabel,
			pair.SameCanonicalDomain,
			pair.LocalIdentityOnly,
			strings.Join(pair.MissingRelationAnchors, "`, `"),
		)
	}
	b.WriteByte('\n')
	return b.String()
}

func runtimeArtifactRelationEndpointLabel(endpoint types.RuntimeArtifactRelationEndpoint) string {
	if path := strings.TrimSpace(endpoint.Path); path != "" && path != "multiple" {
		return path
	}
	return strings.TrimSpace(endpoint.ArtifactID)
}
