// Package probe implements the Layer 2 environment scanner that
// produces a types.EnvFacts snapshot for the env_recommend
// subsystem. The scan is deterministic, cross-platform, and runs
// once per Run (callers cache the result on BusContext).
//
// Design contract: probing never errors fatally. Every detector
// returns its own zero value on failure; partial facts beat no
// facts. The orchestrator's recommendation pipeline tolerates
// missing fields gracefully via nil-checks.
package probe

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Probe runs every probe goroutine in parallel and returns a
// merged types.EnvFacts. Caller passes the repo root so project-
// level scans (pyproject.toml, package.json, etc.) target the
// right directory; probeNetwork can be disabled via opts.
type Options struct {
	RepoRoot     string
	ProbeNetwork bool
}

// Run executes the probe pipeline and returns EnvFacts. The
// returned struct is always non-nil — even on probe-wide failures
// the caller gets ProbedAt + zero-value subfields, so downstream
// callers don't have to nil-check the top struct.
func Run(opts Options) *types.EnvFacts {
	start := time.Now()
	facts := &types.EnvFacts{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		ProbedAt:  start,
		PathTools: map[string]string{},
	}

	var wg sync.WaitGroup
	var mu sync.Mutex // guards facts mutation across goroutines

	wg.Add(4)
	go func() {
		defer wg.Done()
		family, version := probeOSFamily()
		mu.Lock()
		facts.OSFamily = family
		facts.OSVersion = version
		facts.InsideContainer = probeInsideContainer()
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		mu.Lock()
		facts.Shell = probeShell()
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		pms := probePkgManagers()
		mu.Lock()
		facts.PkgManagers = pms
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		files := probeProjectFiles(opts.RepoRoot)
		mu.Lock()
		facts.ProjectFiles = files
		mu.Unlock()
	}()

	wg.Wait()

	// Toolchain probes are sequential because some depend on
	// PathTools / PkgManagers being populated above.
	facts.Pythons = probePythons(facts)
	facts.Nodes = probeNodes(facts)
	facts.Rusts = probeRusts(facts)
	facts.Javas = probeJavas(facts, opts.RepoRoot)
	facts.Rubies = probeRubies(facts)

	if opts.ProbeNetwork {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		facts.Network = probeNetwork(ctx)
	}

	facts.GitRepoState = probeGitRepoState(opts.RepoRoot)

	facts.ProbeDurationMs = time.Since(start).Milliseconds()
	logging.Info("[env/probe] done in %dms (os=%s family=%s pythons=%d nodes=%d pkgmgrs=%d project_files=%d git=%s)",
		facts.ProbeDurationMs, facts.OS, facts.OSFamily,
		len(facts.Pythons), len(facts.Nodes), len(facts.PkgManagers),
		len(facts.ProjectFiles), facts.GitRepoState)
	return facts
}
