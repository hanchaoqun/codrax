package types

import "time"

type PatchEffectEvent struct {
	Code        string `json:"code"`
	Severity    string `json:"severity,omitempty"`
	Path        string `json:"path,omitempty"`
	Message     string `json:"message,omitempty"`
	EvidenceRef string `json:"evidence_ref,omitempty"`
}

type PatchEffectHunk struct {
	OldStart     int `json:"old_start,omitempty"`
	OldLines     int `json:"old_lines,omitempty"`
	NewStart     int `json:"new_start,omitempty"`
	NewLines     int `json:"new_lines,omitempty"`
	AddedLines   int `json:"added_lines,omitempty"`
	RemovedLines int `json:"removed_lines,omitempty"`
}

type PatchEffectFile struct {
	Path         string             `json:"path"`
	OldPath      string             `json:"old_path,omitempty"`
	Status       string             `json:"status,omitempty"`
	Language     string             `json:"language,omitempty"`
	PathRole     SourcePathRole     `json:"path_role,omitempty"`
	AddedLines   int                `json:"added_lines,omitempty"`
	RemovedLines int                `json:"removed_lines,omitempty"`
	Hunks        []PatchEffectHunk  `json:"hunks,omitempty"`
	Events       []PatchEffectEvent `json:"events,omitempty"`
}

// PatchEffectRecord is the typed factual view of the actual applied diff.
// It is lifecycle/evidence metadata and must not participate in
// PlanFingerprint. Hard policy still reads precise fields such as paths,
// fingerprints, parser outcomes, and event codes instead of model prose.
type PatchEffectRecord struct {
	RecordID        string            `json:"record_id,omitempty"`
	PlanID          string            `json:"plan_id,omitempty"`
	SliceID         string            `json:"slice_id,omitempty"`
	Source          string            `json:"source,omitempty"`
	BaseRef         string            `json:"base_ref,omitempty"`
	HeadRef         string            `json:"head_ref,omitempty"`
	DiffFingerprint string            `json:"diff_fingerprint,omitempty"`
	DiffBytes       int               `json:"diff_bytes,omitempty"`
	Files           []PatchEffectFile `json:"files,omitempty"`
	CreatedAt       time.Time         `json:"created_at,omitempty"`
}
