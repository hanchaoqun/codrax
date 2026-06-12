package repomap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/render"
	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

// relation_map typed observation publication.
//
// The relation_map view renders graph-backed relation rows as advisory prose;
// historically that prose was the ONLY carrier — the rows reached downstream
// stages only when the model re-emitted them or a later per-stage graph
// re-probe happened to surface the same edges, and either hop could drop what
// the explorer actually saw. Mirroring trace_query's producer-published
// pattern, the builder below projects the exact rendered rows into
// ledger-ready ObservationRecord rows attached to ToolResult.Observations, so
// the observation ledger compiles them directly without re-parsing the capped
// prose preview.
//
// Classification is the NAVIGATION-fact family, not the citation family: the
// origin is the index origin the observation ledger already assigns repo_map
// facts (cross_repo_index — see answerEvidenceOriginFromStructuredToken's
// "repo_map" token), the role is supporting coverage (the same role family as
// source-inventory lens records), and the grounding policy is the canonical
// (origin, role) mapping — soft, never hard. Rows deliberately do NOT carry a
// current-source origin: the ledger compiler rejects current-source rows from
// producer channels, so index-derived navigation rows can never become
// current-checkout file:line citation evidence.

// relationMapObservationRowCap bounds published relation rows per result. The
// renderer's own top-N cap usually keeps row counts below this; the constant
// is the hard publication ceiling for explicitly widened lenses.
const relationMapObservationRowCap = 50

// relationMapViewData returns the structured relation_map view for typed-row
// extraction, or nil for every other view so Execute keeps the untouched
// GenerateView path (default-stability: non-relation_map views stay
// byte-identical with zero extra render work).
func relationMapViewData(graph *Graph, view string, params ViewParams) *render.ViewData {
	if view != "relation_map" {
		return nil
	}
	return render.GenerateViewData(graph, view, params)
}

// relationMapObservationScope derives the per-result ID namespace for typed
// rows, mirroring trace_query: the stored blob reference is content-hashed and
// unique per distinct rendered output; when no blob was stored (small output,
// no work directory) a digest of the rendered text keeps IDs deterministic and
// distinct across different lenses while staying identical across duplicate
// copies of the same result (ID-level ledger dedup).
func relationMapObservationScope(rawRef, renderedOutput string) string {
	if ref := strings.TrimSpace(rawRef); ref != "" {
		return filepath.Base(ref)
	}
	sum := sha256.Sum256([]byte(renderedOutput))
	return "relation_map-" + hex.EncodeToString(sum[:4])
}

// relationMapTypedObservations maps rendered relation_map rows to typed
// observation rows. Dedup reuses the canonical typed-relation member key
// (relation|source|member|file — the same key family the prompt-side typed
// relation hint merge uses), so the typed side-channel and the typed_graph
// appendix collapse the same member identically; rows whose identity is
// incomplete (empty kind/source/target) are skipped, never weak-keyed.
func relationMapTypedObservations(rows []render.RelationMapRow, rawRef, renderedOutput string, observedAt time.Time) []ctypes.ObservationRecord {
	if len(rows) == 0 {
		return nil
	}
	scope := relationMapObservationScope(rawRef, renderedOutput)
	at := observedAt.Format("2006-01-02T15:04:05Z07:00")
	origin := ctypes.AnswerEvidenceOriginCrossRepoIndex
	role := ctypes.AnswerAggregateRoleSupportingCoverage
	policy := ctypes.AnswerClaimBindingGroundingPolicy(origin, role)
	seen := make(map[string]bool, len(rows))
	var out []ctypes.ObservationRecord
	for _, row := range rows {
		if len(out) >= relationMapObservationRowCap {
			break
		}
		key := ctypes.TypedRelationMemberSurfaceKey(
			ctypes.TypedRelationKind(row.Kind),
			row.Source,
			ctypes.TypedRelationMember{Name: row.Target, File: row.TargetFile},
		)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		span := ctypes.ObservationSpan{}
		if row.ObservedLine > 0 {
			span.LineStart = row.ObservedLine
		}
		out = append(out, ctypes.ObservationRecord{
			ID:              fmt.Sprintf("repo_map:%s#relation_map:%d", scope, len(out)+1),
			Origin:          origin,
			Producer:        "repo_map",
			Role:            role,
			GroundingPolicy: policy,
			SourceRef: ctypes.ObservationSourceRef{
				Kind:   ctypes.ObservationSourceCrossRepoIndex,
				Path:   relationMapObservationPath(row),
				RawRef: strings.TrimSpace(rawRef),
			},
			Span:       span,
			ClaimKey:   fmt.Sprintf("%s %s -> %s", row.Kind, row.Source, row.Target),
			Subject:    row.Source,
			Predicate:  row.Kind,
			Object:     row.Target,
			Summary:    strings.TrimSpace(row.Text),
			ObservedAt: at,
			Confidence: row.Confidence,
		})
	}
	return out
}

func relationMapObservationPath(row render.RelationMapRow) string {
	for _, path := range []string{row.ObservedFile, row.SourceFile, row.TargetFile} {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
