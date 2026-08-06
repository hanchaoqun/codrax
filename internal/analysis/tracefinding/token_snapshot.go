package tracefinding

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// RegistryHash returns a stable hash of the live causal token registry.
func RegistryHash() string {
	universe := tracequery.CausalTokenUniverse()
	lines := make([]string, 0, len(universe))
	for _, token := range universe {
		spec, ok := tracequery.CausalTokenSpecFor(token)
		if !ok {
			continue
		}
		dir := string(tracequery.CausalTokenFixDirectionFor(token))
		lines = append(lines, fmt.Sprintf("%s|%s|%s|%s|%t|%s",
			token, spec.Lane, spec.Additivity, spec.Subject, spec.RowToken, dir))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// SnapshotToken builds an immutable registry snapshot for a token.
func SnapshotToken(token string) (types.TraceCausalTokenSnapshot, error) {
	token = strings.TrimSpace(token)
	spec, ok := tracequery.CausalTokenSpecFor(token)
	if !ok {
		return types.TraceCausalTokenSnapshot{}, fmt.Errorf("unknown causal token %q", token)
	}
	return types.TraceCausalTokenSnapshot{
		Token:        token,
		Lane:         string(spec.Lane),
		Additivity:   string(spec.Additivity),
		SubjectKind:  string(spec.Subject),
		FixDirection: string(tracequery.CausalTokenFixDirectionFor(token)),
		RegistryHash: RegistryHash(),
	}, nil
}
