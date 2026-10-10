package types

import "encoding/json"

// A persisted preview cannot stand in for the complete source selection on
// resume. Fingerprint the prepared source identities and full-content digests;
// querying still independently revalidates each live source generation.
func readRunLogAttachmentFingerprint(ctx *BusContext) ReadRunAttachmentFingerprint {
	if ctx.AttachedLogCatalog == nil {
		return ReadRunAttachmentFingerprintFromPayload(ReadRunAttachmentKindLog, ctx.AttachedLog, "")
	}
	metadata, err := json.Marshal(ctx.AttachedLogCatalog.Sources())
	if err != nil {
		return ReadRunAttachmentFingerprint{Kind: ReadRunAttachmentKindLog, ReasonCode: ReadRunFingerprintReasonUnavailable}
	}
	return ReadRunAttachmentFingerprintFromPayload(ReadRunAttachmentKindLog, string(metadata), "complete_log_catalog")
}
