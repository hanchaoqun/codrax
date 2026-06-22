package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

const (
	ReadRunRepoFingerprintKindGitHead = "git_head"
	ReadRunEnvironmentKindCodrax      = "codrax_runtime"

	ReadRunFingerprintReasonUnavailable = "unavailable"
	ReadRunFingerprintReasonNotGit      = "not_git"
	ReadRunFingerprintReasonNotFound    = "not_found"

	ReadRunAttachmentKindLog   = "log"
	ReadRunAttachmentKindTrace = "trace"

	ReadRunConfigFingerprintSearchExcludeRoots = "search_exclude_roots"
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

// ReadRunEnvironmentFingerprint captures bounded runtime/tool/config identity
// for explicit read-run resume. Missing or unavailable values are audit-only;
// hard gates may compare only fields that are precise on both sides.
type ReadRunEnvironmentFingerprint struct {
	Kind            string                     `json:"kind,omitempty"`
	Available       bool                       `json:"available,omitempty"`
	CodraxVersion   string                     `json:"codrax_version,omitempty"`
	CodraxBuildTime string                     `json:"codrax_build_time,omitempty"`
	GoVersion       string                     `json:"go_version,omitempty"`
	GOOS            string                     `json:"goos,omitempty"`
	GOARCH          string                     `json:"goarch,omitempty"`
	Tools           []ReadRunToolFingerprint   `json:"tool_fingerprints,omitempty"`
	Configs         []ReadRunConfigFingerprint `json:"config_fingerprints,omitempty"`
	ReasonCode      string                     `json:"reason_code,omitempty"`
}

type ReadRunToolFingerprint struct {
	Name        string `json:"name,omitempty"`
	Available   bool   `json:"available,omitempty"`
	Executable  string `json:"executable,omitempty"`
	VersionHash string `json:"version_hash,omitempty"`
	FileHash    string `json:"file_hash,omitempty"`
	ReasonCode  string `json:"reason_code,omitempty"`
}

type ReadRunConfigFingerprint struct {
	Name       string `json:"name,omitempty"`
	Available  bool   `json:"available,omitempty"`
	Hash       string `json:"hash,omitempty"`
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

func NormalizeReadRunEnvironmentFingerprint(in ReadRunEnvironmentFingerprint) ReadRunEnvironmentFingerprint {
	out := ReadRunEnvironmentFingerprint{
		Kind:            strings.TrimSpace(in.Kind),
		Available:       in.Available,
		CodraxVersion:   strings.TrimSpace(in.CodraxVersion),
		CodraxBuildTime: strings.TrimSpace(in.CodraxBuildTime),
		GoVersion:       strings.TrimSpace(in.GoVersion),
		GOOS:            strings.TrimSpace(in.GOOS),
		GOARCH:          strings.TrimSpace(in.GOARCH),
		Tools:           NormalizeReadRunToolFingerprints(in.Tools),
		Configs:         NormalizeReadRunConfigFingerprints(in.Configs),
		ReasonCode:      strings.TrimSpace(in.ReasonCode),
	}
	if out.Kind == "" && (out.CodraxVersion != "" || out.CodraxBuildTime != "" || out.GoVersion != "" || out.GOOS != "" || out.GOARCH != "" || len(out.Tools) > 0 || len(out.Configs) > 0) {
		out.Kind = ReadRunEnvironmentKindCodrax
	}
	if out.Kind == "" && !out.Available && out.CodraxVersion == "" && out.CodraxBuildTime == "" && out.GoVersion == "" && out.GOOS == "" && out.GOARCH == "" && len(out.Tools) == 0 && len(out.Configs) == 0 && out.ReasonCode == "" {
		return ReadRunEnvironmentFingerprint{}
	}
	if out.CodraxVersion == "" && out.CodraxBuildTime == "" && out.GoVersion == "" && out.GOOS == "" && out.GOARCH == "" && len(out.Tools) == 0 && len(out.Configs) == 0 {
		out.Available = false
		if out.ReasonCode == "" {
			out.ReasonCode = ReadRunFingerprintReasonUnavailable
		}
		return out
	}
	out.Available = out.Available || out.Kind != ""
	return out
}

func NormalizeReadRunToolFingerprint(in ReadRunToolFingerprint) ReadRunToolFingerprint {
	out := ReadRunToolFingerprint{
		Name:        strings.TrimSpace(in.Name),
		Available:   in.Available,
		Executable:  strings.TrimSpace(in.Executable),
		VersionHash: strings.TrimSpace(in.VersionHash),
		FileHash:    strings.TrimSpace(in.FileHash),
		ReasonCode:  strings.TrimSpace(in.ReasonCode),
	}
	if out.Name == "" {
		return ReadRunToolFingerprint{}
	}
	if out.Executable == "" && out.VersionHash == "" && out.FileHash == "" {
		out.Available = false
		if out.ReasonCode == "" {
			out.ReasonCode = ReadRunFingerprintReasonUnavailable
		}
	} else {
		out.Available = true
	}
	return out
}

func NormalizeReadRunToolFingerprints(in []ReadRunToolFingerprint) []ReadRunToolFingerprint {
	if len(in) == 0 {
		return nil
	}
	byName := make(map[string]ReadRunToolFingerprint, len(in))
	for _, item := range in {
		item = NormalizeReadRunToolFingerprint(item)
		if item.Name == "" {
			continue
		}
		if _, exists := byName[item.Name]; exists {
			continue
		}
		byName[item.Name] = item
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]ReadRunToolFingerprint, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func NormalizeReadRunConfigFingerprint(in ReadRunConfigFingerprint) ReadRunConfigFingerprint {
	out := ReadRunConfigFingerprint{
		Name:       strings.TrimSpace(in.Name),
		Available:  in.Available,
		Hash:       strings.TrimSpace(in.Hash),
		ReasonCode: strings.TrimSpace(in.ReasonCode),
	}
	if out.Name == "" {
		return ReadRunConfigFingerprint{}
	}
	if out.Hash == "" {
		out.Available = false
		if out.ReasonCode == "" {
			out.ReasonCode = ReadRunFingerprintReasonUnavailable
		}
	} else {
		out.Available = true
	}
	return out
}

func NormalizeReadRunConfigFingerprints(in []ReadRunConfigFingerprint) []ReadRunConfigFingerprint {
	if len(in) == 0 {
		return nil
	}
	byName := make(map[string]ReadRunConfigFingerprint, len(in))
	for _, item := range in {
		item = NormalizeReadRunConfigFingerprint(item)
		if item.Name == "" {
			continue
		}
		if _, exists := byName[item.Name]; exists {
			continue
		}
		byName[item.Name] = item
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]ReadRunConfigFingerprint, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func ReadRunConfigFingerprintFromStringSlice(name string, values []string) ReadRunConfigFingerprint {
	name = strings.TrimSpace(name)
	if name == "" {
		return ReadRunConfigFingerprint{}
	}
	normalized := compactReadRunStrings(values...)
	data, err := json.Marshal(normalized)
	if err != nil {
		return NormalizeReadRunConfigFingerprint(ReadRunConfigFingerprint{Name: name, ReasonCode: ReadRunFingerprintReasonUnavailable})
	}
	return NormalizeReadRunConfigFingerprint(ReadRunConfigFingerprint{
		Name:      name,
		Available: true,
		Hash:      readRunSHA256Hex(name + "\x00" + string(data)),
	})
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

func ReadRunEnvironmentFingerprintsEqual(a, b ReadRunEnvironmentFingerprint) bool {
	a = NormalizeReadRunEnvironmentFingerprint(a)
	b = NormalizeReadRunEnvironmentFingerprint(b)
	if !a.Available || !b.Available {
		return true
	}
	if comparableStringMismatch(a.Kind, b.Kind) ||
		comparableStringMismatch(a.CodraxVersion, b.CodraxVersion) ||
		comparableStringMismatch(a.CodraxBuildTime, b.CodraxBuildTime) ||
		comparableStringMismatch(a.GoVersion, b.GoVersion) ||
		comparableStringMismatch(a.GOOS, b.GOOS) ||
		comparableStringMismatch(a.GOARCH, b.GOARCH) {
		return false
	}
	if !readRunToolFingerprintsEqual(a.Tools, b.Tools) {
		return false
	}
	if !readRunConfigFingerprintsEqual(a.Configs, b.Configs) {
		return false
	}
	return true
}

func readRunToolFingerprintsEqual(a, b []ReadRunToolFingerprint) bool {
	left := readRunToolFingerprintMap(a)
	right := readRunToolFingerprintMap(b)
	for name, l := range left {
		r, ok := right[name]
		if !ok || !l.Available || !r.Available {
			continue
		}
		if comparableStringMismatch(l.Executable, r.Executable) ||
			comparableStringMismatch(l.VersionHash, r.VersionHash) ||
			comparableStringMismatch(l.FileHash, r.FileHash) {
			return false
		}
	}
	return true
}

func readRunConfigFingerprintsEqual(a, b []ReadRunConfigFingerprint) bool {
	left := readRunConfigFingerprintMap(a)
	right := readRunConfigFingerprintMap(b)
	for name, l := range left {
		r, ok := right[name]
		if !ok || !l.Available || !r.Available {
			continue
		}
		if comparableStringMismatch(l.Hash, r.Hash) {
			return false
		}
	}
	return true
}

func readRunToolFingerprintMap(items []ReadRunToolFingerprint) map[string]ReadRunToolFingerprint {
	out := map[string]ReadRunToolFingerprint{}
	for _, item := range NormalizeReadRunToolFingerprints(items) {
		if item.Name != "" {
			out[item.Name] = item
		}
	}
	return out
}

func readRunConfigFingerprintMap(items []ReadRunConfigFingerprint) map[string]ReadRunConfigFingerprint {
	out := map[string]ReadRunConfigFingerprint{}
	for _, item := range NormalizeReadRunConfigFingerprints(items) {
		if item.Name != "" {
			out[item.Name] = item
		}
	}
	return out
}

func comparableStringMismatch(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	return a != "" && b != "" && a != b
}

func readRunSHA256Hex(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
