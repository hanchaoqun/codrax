package types

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

const (
	ReadRunRepoFingerprintKindGitHead = "git_head"

	ReadRunFingerprintReasonUnavailable = "unavailable"
	ReadRunFingerprintReasonNotGit      = "not_git"

	ReadRunAttachmentKindLog   = "log"
	ReadRunAttachmentKindTrace = "trace"
)

// ReadRunRepoFingerprint captures precise repository identity available at
// snapshot time. Empty or Available=false means audit metadata only; resume
// gates must hard-reject only when both sides have comparable precise values.
type ReadRunRepoFingerprint struct {
	Kind       string `json:"kind,omitempty"`
	Available  bool   `json:"available,omitempty"`
	Head       string `json:"head,omitempty"`
	StatusHash string `json:"status_hash,omitempty"`
	ReasonCode string `json:"reason_code,omitempty"`
}

// ReadRunAttachmentFingerprint identifies attached runtime artifacts without
// storing the raw attachment body in the snapshot.
type ReadRunAttachmentFingerprint struct {
	Kind       string `json:"kind,omitempty"`
	Present    bool   `json:"present,omitempty"`
	Hash       string `json:"hash,omitempty"`
	Bytes      int    `json:"bytes,omitempty"`
	Source     string `json:"source,omitempty"`
	ReasonCode string `json:"reason_code,omitempty"`
}

func ReadRunRequestHash(request string) string {
	request = strings.TrimSpace(request)
	if request == "" {
		return ""
	}
	return readRunSHA256Hex("request\x00" + request)
}

func NormalizeReadRunRepoFingerprint(in ReadRunRepoFingerprint) ReadRunRepoFingerprint {
	out := ReadRunRepoFingerprint{
		Kind:       strings.TrimSpace(in.Kind),
		Available:  in.Available,
		Head:       strings.TrimSpace(in.Head),
		StatusHash: strings.TrimSpace(in.StatusHash),
		ReasonCode: strings.TrimSpace(in.ReasonCode),
	}
	if out.Kind == "" && (out.Head != "" || out.StatusHash != "") {
		out.Kind = ReadRunRepoFingerprintKindGitHead
	}
	if out.Head == "" {
		out.Available = false
		if out.ReasonCode == "" && out.Kind != "" {
			out.ReasonCode = ReadRunFingerprintReasonUnavailable
		}
	}
	if out.Kind == "" && !out.Available && out.Head == "" && out.StatusHash == "" && out.ReasonCode == "" {
		return ReadRunRepoFingerprint{}
	}
	return out
}

func NormalizeReadRunAttachmentFingerprint(in ReadRunAttachmentFingerprint) ReadRunAttachmentFingerprint {
	out := ReadRunAttachmentFingerprint{
		Kind:       strings.TrimSpace(in.Kind),
		Present:    in.Present,
		Hash:       strings.TrimSpace(in.Hash),
		Bytes:      in.Bytes,
		Source:     strings.TrimSpace(in.Source),
		ReasonCode: strings.TrimSpace(in.ReasonCode),
	}
	if out.Bytes < 0 {
		out.Bytes = 0
	}
	if out.Hash == "" {
		out.Present = false
		if out.ReasonCode == "" && out.Kind != "" {
			out.ReasonCode = ReadRunFingerprintReasonUnavailable
		}
	}
	if out.Kind == "" && !out.Present && out.Hash == "" && out.Bytes == 0 && out.Source == "" && out.ReasonCode == "" {
		return ReadRunAttachmentFingerprint{}
	}
	return out
}

func NormalizeReadRunAttachmentFingerprints(in []ReadRunAttachmentFingerprint) []ReadRunAttachmentFingerprint {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]ReadRunAttachmentFingerprint, 0, len(in))
	for _, item := range in {
		item = NormalizeReadRunAttachmentFingerprint(item)
		if item.Kind == "" {
			continue
		}
		key := item.Kind
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Kind < out[j].Kind
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func ReadRunAttachmentFingerprintsFromBusContext(ctx *BusContext) []ReadRunAttachmentFingerprint {
	if ctx == nil {
		return nil
	}
	var out []ReadRunAttachmentFingerprint
	if item := ReadRunAttachmentFingerprintFromPayload(ReadRunAttachmentKindLog, ctx.AttachedLog, ""); item.Kind != "" {
		out = append(out, item)
	}
	if item := ReadRunAttachmentFingerprintFromPayload(ReadRunAttachmentKindTrace, ctx.AttachedHitrace, ctx.AttachedHitraceSource); item.Kind != "" {
		out = append(out, item)
	}
	return NormalizeReadRunAttachmentFingerprints(out)
}

func ReadRunAttachmentFingerprintFromPayload(kind, payload, source string) ReadRunAttachmentFingerprint {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return ReadRunAttachmentFingerprint{}
	}
	if payload == "" {
		return ReadRunAttachmentFingerprint{}
	}
	return NormalizeReadRunAttachmentFingerprint(ReadRunAttachmentFingerprint{
		Kind:    kind,
		Present: true,
		Hash:    readRunSHA256Hex(kind + "\x00" + payload),
		Bytes:   len([]byte(payload)),
		Source:  source,
	})
}

func ReadRunAttachmentFingerprintByKind(items []ReadRunAttachmentFingerprint, kind string) (ReadRunAttachmentFingerprint, bool) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return ReadRunAttachmentFingerprint{}, false
	}
	for _, item := range NormalizeReadRunAttachmentFingerprints(items) {
		if item.Kind == kind {
			return item, true
		}
	}
	return ReadRunAttachmentFingerprint{}, false
}

func ReadRunFingerprintsComparable(a, b ReadRunRepoFingerprint) bool {
	a = NormalizeReadRunRepoFingerprint(a)
	b = NormalizeReadRunRepoFingerprint(b)
	return a.Available && b.Available && a.Kind != "" && a.Kind == b.Kind && a.Head != "" && b.Head != ""
}

func ReadRunRepoFingerprintsEqual(a, b ReadRunRepoFingerprint) bool {
	if !ReadRunFingerprintsComparable(a, b) {
		return true
	}
	a = NormalizeReadRunRepoFingerprint(a)
	b = NormalizeReadRunRepoFingerprint(b)
	return a.Head == b.Head && a.StatusHash == b.StatusHash
}

func ReadRunAttachmentFingerprintsEqual(a, b ReadRunAttachmentFingerprint) bool {
	a = NormalizeReadRunAttachmentFingerprint(a)
	b = NormalizeReadRunAttachmentFingerprint(b)
	if !a.Present && !b.Present {
		return true
	}
	return a.Present == b.Present &&
		a.Kind == b.Kind &&
		a.Hash == b.Hash &&
		a.Bytes == b.Bytes &&
		a.Source == b.Source
}

func readRunSHA256Hex(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
