package tool

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runnerResultScopeLabel is the existing report producer's spelling, shared
// with its consumer. This is not testSurfaceCandidateKey: only Python/Java
// publish the framework in their label. Keep already-published bytes stable.
func runnerResultScopeLabel(runner, framework, workingDir string) string {
	if (runner == "python" || runner == "java") && strings.TrimSpace(framework) != "" {
		runner += "/" + strings.TrimSpace(framework)
	}
	return runner + "@" + workingDir
}

type projectTestResultScope struct {
	prefix    string
	key       string
	ambiguous bool
}

// The catalog comes only from this report's typed execution scopes. A raw
// pytest suite can itself contain "::", so a delimiter is not scope evidence.
// This restores candidate-scope ownership only. Invocation identity is joined
// independently, so the same candidate can execute repeatedly without borrowing
// an earlier command's exit status or another command's assertion rows.
func projectTestResultScopeCatalog(report *types.ChangeReport) []projectTestResultScope {
	byPrefix := map[string]projectTestResultScope{}
	add := func(runner, framework, workingDir string) {
		if runner == "" || runner == "verification_probe" {
			return
		}
		workingDir = strings.TrimSpace(workingDir)
		if workingDir == "" || workingDir == "." {
			return // Root results retain their native, unqualified identity.
		}
		prefix := runnerResultScopeLabel(runner, framework, workingDir) + "::"
		key := testSurfaceCandidateKey(runner, framework, workingDir)
		if prior, ok := byPrefix[prefix]; ok {
			prior.ambiguous = prior.ambiguous || prior.key != key
			byPrefix[prefix] = prior
		} else {
			byPrefix[prefix] = projectTestResultScope{prefix: prefix, key: key}
		}
	}
	if report.TestSurface != nil {
		for _, candidate := range report.TestSurface.Candidates {
			add(candidate.Runner, candidate.Framework, candidate.WorkingDir)
		}
	}
	for _, command := range report.ExecutedCommands {
		if strings.TrimSpace(command.Outcome) == types.ExecutedCommandOutcomeExecuted {
			add(command.Runner, command.Framework, command.WorkingDir)
		}
	}
	scopes := make([]projectTestResultScope, 0, len(byPrefix))
	for _, scope := range byPrefix {
		scopes = append(scopes, scope)
	}
	sort.Slice(scopes, func(i, j int) bool { return scopes[i].prefix < scopes[j].prefix })
	return scopes
}

// Both identity fields must belong to one unambiguous scope. Strip its prefix
// exactly once, and only for path membership; the caller still compares the
// original complete suite/ID with the declaration. No result is rewritten.
func projectTestResultSuiteForCandidate(candidate types.TestSurfaceCandidate, result types.TestResult, scopes []projectTestResultScope) (string, bool) {
	localSuite, suiteKey, suiteOK := projectTestResultLocalIdentity(result.Suite, scopes)
	_, idKey, idOK := projectTestResultLocalIdentity(result.AssertionID, scopes)
	if !suiteOK || !idOK || suiteKey != idKey {
		return "", false
	}
	workingDir := strings.TrimSpace(candidate.WorkingDir)
	if workingDir == "" || workingDir == "." {
		return localSuite, suiteKey == ""
	}
	return localSuite, suiteKey == testSurfaceCandidateKey(candidate.Runner, candidate.Framework, workingDir)
}

func projectTestResultLocalIdentity(value string, scopes []projectTestResultScope) (string, string, bool) {
	value = strings.TrimSpace(value)
	var match *projectTestResultScope
	for i := range scopes {
		if !strings.HasPrefix(value, scopes[i].prefix) {
			continue
		}
		// Labels are not escaped; a directory can contain "::". Multiple
		// matching prefixes are ambiguous, not a longest-prefix authority.
		if match != nil || scopes[i].ambiguous {
			return "", "", false
		}
		match = &scopes[i]
	}
	if match == nil {
		return value, "", value != ""
	}
	local := strings.TrimPrefix(value, match.prefix)
	if local == "" {
		return "", "", false
	}
	for _, scope := range scopes {
		if strings.HasPrefix(local, scope.prefix) {
			return "", "", false // Never peel repeated or mixed qualifiers.
		}
	}
	return local, match.key, true
}
