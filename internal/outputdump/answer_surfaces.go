package outputdump

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
)

const AnswerSurfacesExt = ".answer-surfaces.json"

// AnswerSurfaceArtifact is an observation-only eval receipt, not an answer,
// grounding certificate or model schema. MarkdownSHA256 binds it to the exact
// persisted transcript; AnswerSHA256 identifies its final answer component.
type AnswerSurfaceArtifact struct {
	SchemaVersion  int    `json:"schema_version"`
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	MarkdownSHA256 string `json:"markdown_sha256"`
	AnswerSHA256   string `json:"answer_sha256"`
	Primary        string `json:"primary"`
	Principal      string `json:"principal"`
}

func AnswerSurfacesPathForMarkdown(path string) string {
	if !strings.HasSuffix(path, Ext) {
		return ""
	}
	return strings.TrimSuffix(path, Ext) + AnswerSurfacesExt
}

func answerSurfaceDigest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func writeAnswerSurfaces(a Args, markdownPath, body string) {
	r := AnswerSurfaceArtifact{SchemaVersion: 1, Status: "unavailable",
		Reason: "final_render_ownership_unavailable", MarkdownSHA256: answerSurfaceDigest(body), AnswerSHA256: answerSurfaceDigest(a.Answer)}
	if s := a.AnswerSurfaces; s != nil && s.Answer == a.Answer {
		r.Status, r.Reason, r.Primary, r.Principal = "available", "", s.Primary, s.Principal
	}
	path := AnswerSurfacesPathForMarkdown(markdownPath)
	raw, err := json.Marshal(r)
	if err == nil {
		raw = append(raw, '\n')
		err = os.WriteFile(path, raw, 0o644)
	}
	if err != nil {
		logging.Warning("[output_dump] answer surfaces write failed: %v", err)
		return // Never affect product answer delivery or root-cause artifacts.
	}
	logging.Info("[output_dump] answer surfaces path=%s sha256=%s", path, answerSurfaceDigest(string(raw)))
}
