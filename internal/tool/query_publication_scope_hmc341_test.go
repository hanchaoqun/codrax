package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC341PublicationCoordinateKeepsCompleteQueryAndSourceOwnership(t *testing.T) {
	_, result := businessNativeSelfQuery339(t, 5, 5.012)
	record, _, _ := businessNativeRows339(t, result, "business-700", "OpenDocument")
	ref := record.SourceRef
	before, _ := json.Marshal(ref)
	scope := traceQueryObservationPublicationScope(ref)
	digest := sha256.Sum256(before)
	if scope != "publication-"+hex.EncodeToString(digest[:]) || strings.Contains(scope, ref.QueryScopeID) {
		t.Fatal("coordinate must use a full digest, not repeat or truncate the source receipt")
	}
	// The public record has since received per-row physical clock provenance.
	// Consumers must resolve its opaque ID, not regenerate the producer's key
	// from a later enriched receipt. Check the complete digest shape instead.
	publicScope, _, _ := strings.Cut(strings.TrimPrefix(record.ID, "trace_query:publication-"), "#")
	decoded, err := hex.DecodeString(publicScope)
	if !strings.HasPrefix(record.ID, "trace_query:publication-") || err != nil || len(decoded) != sha256.Size {
		t.Fatal("public native coordinate repeated query metadata or truncated the digest")
	}
	if len(ref.QueryScopeID) <= len(scope) || ref.PayloadRef == "" || ref.RawRef == "" || !ref.QueryWindowKnown {
		t.Fatal("compact ID erased the independent full source/query receipt")
	}
	for name, change := range map[string]func(*types.ObservationSourceRef){
		"query":        func(r *types.ObservationSourceRef) { r.QueryScopeID += ":other" },
		"owner":        func(r *types.ObservationSourceRef) { r.ArtifactID += "-other" },
		"source":       func(r *types.ObservationSourceRef) { r.Path += ".other" },
		"payload":      func(r *types.ObservationSourceRef) { r.PayloadRef += ".other" },
		"raw":          func(r *types.ObservationSourceRef) { r.RawRef += ".other" },
		"query window": func(r *types.ObservationSourceRef) { r.QueryWindowEndTs += 1 },
		"query target": func(r *types.ObservationSourceRef) { r.QueryTargetPID++ },
	} {
		t.Run(name, func(t *testing.T) {
			other := ref
			change(&other)
			if traceQueryObservationPublicationScope(other) == scope {
				t.Fatal("different complete source/query receipt borrowed another coordinate")
			}
		})
	}
	after, _ := json.Marshal(ref)
	if !reflect.DeepEqual(before, after) || traceQueryObservationPublicationScope(ref) != scope {
		t.Fatal("coordinate generation mutated the receipt or made exact repetition unstable")
	}
	ref.QueryScopeID = ""
	if traceQueryObservationPublicationScope(ref) != "" {
		t.Fatal("query-less legacy source acquired a fresh query namespace")
	}
}
