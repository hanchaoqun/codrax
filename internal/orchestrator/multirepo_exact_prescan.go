package orchestrator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	multiRepoExactFocusMaxTokens     = 8
	multiRepoExactFocusMaxFiles      = 10000
	multiRepoExactFocusMaxBytes      = 32 * 1024 * 1024
	multiRepoExactFocusMaxFileBytes  = 1024 * 1024
	multiRepoExactFocusMaxSamples    = 3
	multiRepoExactFocusMinTokenBytes = 3
)

var (
	multiRepoExactFocusTokenRE = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:(?:\.|::|#|-)[A-Za-z_][A-Za-z0-9_]*)*`)
	errExactFocusBudget        = errors.New("multi-repo exact focus pre-scan budget exhausted")
)

type exactFocusBudget struct {
	files int
	bytes int64
}

type exactFocusRepoHit struct {
	slug    string
	rootRel string
	tokens  map[string]bool
	samples map[string][]string
}

// tryExactMultiRepoFocus performs a bounded exact-literal source/config scan
// before the model-only multi-repo selector. It does not infer intent from
// prose. It only treats code-shaped request literals that occur in sub-repo
// files as routing evidence. If the scan cannot complete within budget, or the
// hit set exceeds the active cap, the caller should continue with the existing
// selector/fallback path.
func tryExactMultiRepoFocus(mg *multigraph.MultiGraph, request string) *types.MultiRepoFocusDecision {
	if mg == nil || mg.IsSingle() || mg.Topology() == nil || len(mg.Topology().Repos) < 2 {
		return nil
	}
	tokens := extractMultiRepoExactFocusTokens(request)
	if len(tokens) == 0 {
		return nil
	}
	budget := &exactFocusBudget{}
	hits := make([]exactFocusRepoHit, 0, len(mg.Topology().Repos))
	complete := true
	for _, sr := range mg.Topology().Repos {
		repoHit, ok, repoComplete := scanSubRepoForExactFocusTokens(sr.RootAbs, sr.RootRel, sr.Slug, tokens, budget)
		if !repoComplete {
			complete = false
		}
		if ok {
			hits = append(hits, repoHit)
		}
	}
	if !complete {
		logging.Info("multi-repo: exact focus pre-scan skipped routing because scan budget was exhausted")
		return nil
	}
	if len(hits) == 0 {
		return nil
	}
	if len(hits) > mg.Cap() {
		logging.Info("multi-repo: exact focus pre-scan found %d hit sub-repos, over cap=%d; deferring to selector", len(hits), mg.Cap())
		return nil
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if len(hits[i].tokens) != len(hits[j].tokens) {
			return len(hits[i].tokens) > len(hits[j].tokens)
		}
		return hits[i].rootRel < hits[j].rootRel
	})
	candidates := make([]types.MultiRepoFocusCandidate, 0, len(hits))
	rootRels := make([]string, 0, len(hits))
	for _, hit := range hits {
		rootRels = append(rootRels, hit.rootRel)
		candidates = append(candidates, types.MultiRepoFocusCandidate{
			RootRel:    hit.rootRel,
			Slug:       hit.slug,
			Confidence: 0.95,
			Reason:     exactFocusHitReason(hit),
			Source:     types.MultiRepoFocusSourceExactPrescan,
		})
	}
	logging.Info("multi-repo: exact focus pre-scan selected %d sub-repo(s): %s", len(rootRels), strings.Join(rootRels, ", "))
	return &types.MultiRepoFocusDecision{
		Source:     types.MultiRepoFocusSourceExactPrescan,
		Confidence: 0.95,
		Candidates: candidates,
		Rationale:  "exact request literal(s) were found in source/config-like files before topology-only selection",
	}
}

func extractMultiRepoExactFocusTokens(request string) []string {
	request = strings.TrimSpace(request)
	if request == "" {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]string, 0, multiRepoExactFocusMaxTokens)
	for _, raw := range multiRepoExactFocusTokenRE.FindAllString(request, -1) {
		token := strings.Trim(raw, " \t\r\n\"'`.,;:!?()[]{}<>")
		if !isExactFocusToken(token) {
			continue
		}
		key := strings.ToLower(token)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, token)
		if len(out) >= multiRepoExactFocusMaxTokens {
			break
		}
	}
	return out
}

func isExactFocusToken(token string) bool {
	if len(token) < multiRepoExactFocusMinTokenBytes {
		return false
	}
	var hasLower, hasUpper, hasDigit bool
	for _, r := range token {
		if unicode.IsLower(r) {
			hasLower = true
		}
		if unicode.IsUpper(r) {
			hasUpper = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if strings.ContainsAny(token, "_./#:-") {
		return true
	}
	if hasLower && hasUpper {
		return true
	}
	if (hasLower || hasUpper) && hasDigit {
		return true
	}
	return false
}

func scanSubRepoForExactFocusTokens(rootAbs, rootRel, slug string, tokens []string, budget *exactFocusBudget) (exactFocusRepoHit, bool, bool) {
	hit := exactFocusRepoHit{
		slug:    slug,
		rootRel: rootRel,
		tokens:  make(map[string]bool),
		samples: make(map[string][]string),
	}
	if rootAbs == "" || len(tokens) == 0 {
		return hit, false, true
	}
	complete := true
	err := filepath.WalkDir(rootAbs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || tool.IsExcludedDirName(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(rootAbs, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !isExactFocusSearchFile(rel) {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if info.Size() > multiRepoExactFocusMaxFileBytes {
			complete = false
			return errExactFocusBudget
		}
		budget.files++
		budget.bytes += info.Size()
		if budget.files > multiRepoExactFocusMaxFiles || budget.bytes > multiRepoExactFocusMaxBytes {
			complete = false
			return errExactFocusBudget
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil || len(data) == 0 || bytesContainNUL(data) {
			return nil
		}
		text := string(data)
		for _, token := range tokens {
			if !strings.Contains(text, token) {
				continue
			}
			hit.tokens[token] = true
			if len(hit.samples[token]) < multiRepoExactFocusMaxSamples {
				hit.samples[token] = append(hit.samples[token], rel)
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errExactFocusBudget) {
		return hit, len(hit.tokens) > 0, complete
	}
	return hit, len(hit.tokens) > 0, complete
}

func isExactFocusSearchFile(rel string) bool {
	if rmtypes.DetectLanguage(rel) != "" {
		return true
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".json", ".yaml", ".yml", ".toml", ".xml", ".properties", ".gradle", ".kts":
		return true
	default:
		return false
	}
}

func bytesContainNUL(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func exactFocusHitReason(hit exactFocusRepoHit) string {
	if len(hit.tokens) == 0 {
		return ""
	}
	tokens := make([]string, 0, len(hit.tokens))
	for token := range hit.tokens {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		samples := hit.samples[token]
		if len(samples) == 0 {
			parts = append(parts, token)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s in %s", token, strings.Join(samples, ", ")))
	}
	return "exact literal hit: " + strings.Join(parts, "; ")
}
