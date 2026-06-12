package repomap

import (
	"strings"
	"testing"
	"time"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func bannerTestGraph(status rmtypes.RepoMapIndexStatus, fallback int) *Graph {
	return &Graph{
		Metadata: rmtypes.Metadata{
			ScanTime:          time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
			FileCount:         12,
			SymbolCount:       80,
			RelationCount:     200,
			FallbackFileCount: fallback,
			IndexStatus:       status,
		},
	}
}

func TestIndexStatusBanner_FreshCacheHitIsQuietAndLedgerParseable(t *testing.T) {
	g := bannerTestGraph(rmtypes.RepoMapIndexStatus{
		Source:         rmtypes.IndexSourceCacheHit,
		Freshness:      rmtypes.IndexFreshnessFresh,
		CacheWrittenAt: "2026-06-12T09:00:00Z",
	}, 0)
	out := prependRepoMapIndexStatusBanner(g, "BODY")
	head := strings.SplitN(out, "\n", 2)[0]
	if !strings.HasPrefix(head, "[Repo Map Index: ") || !strings.HasSuffix(head, "]") {
		t.Fatalf("head line not ledger-banner shaped: %q", head)
	}
	for _, want := range []string{"source=cache_hit", "files=12", "symbols=80", "relations=200", "cache_written=2026-06-12T09:00:00Z"} {
		if !strings.Contains(head, want) {
			t.Errorf("head missing %q: %q", want, head)
		}
	}
	// Fresh + zero fallback: NO warning or verify lines (anti-noise).
	for _, forbid := range []string{"rebuilt", "parse fallback", "verify selected files"} {
		if strings.Contains(out, forbid) {
			t.Errorf("fresh banner must not carry %q:\n%s", forbid, out)
		}
	}
	if !strings.HasSuffix(out, "BODY") {
		t.Errorf("body not preserved")
	}
}

func TestIndexStatusBanner_RebuildReasonRendered(t *testing.T) {
	g := bannerTestGraph(rmtypes.RepoMapIndexStatus{
		Source:         rmtypes.IndexSourceFullScan,
		Freshness:      rmtypes.IndexFreshnessFresh,
		RebuildReasons: []string{"schema_version_mismatch"},
	}, 0)
	out := prependRepoMapIndexStatusBanner(g, "BODY")
	if !strings.Contains(out, "rebuilt=schema_version_mismatch") {
		t.Errorf("missing typed rebuild reason in banner head:\n%s", out)
	}
	if !strings.Contains(out, "previous index cache was incompatible") {
		t.Errorf("missing rebuild note:\n%s", out)
	}
}

func TestIndexStatusBanner_FallbackCountWarnsAndAsksVerification(t *testing.T) {
	g := bannerTestGraph(rmtypes.RepoMapIndexStatus{
		Source:    rmtypes.IndexSourceFullScan,
		Freshness: rmtypes.IndexFreshnessFresh,
	}, 23)
	out := prependRepoMapIndexStatusBanner(g, "BODY")
	if !strings.Contains(out, "fallback_files=23") {
		t.Errorf("missing fallback_files in head:\n%s", out)
	}
	if !strings.Contains(out, "parse fallback: 23 files") {
		t.Errorf("missing fallback prose line:\n%s", out)
	}
	if !strings.Contains(out, "verify selected files with read_file or targeted grep") {
		t.Errorf("missing verify reminder:\n%s", out)
	}
}

func TestIndexStatusBanner_InMemoryReuseAsksVerification(t *testing.T) {
	g := bannerTestGraph(rmtypes.RepoMapIndexStatus{
		Source:    rmtypes.IndexSourceInMemoryReuse,
		Freshness: rmtypes.IndexFreshnessReused,
	}, 0)
	out := prependRepoMapIndexStatusBanner(g, "BODY")
	if !strings.Contains(out, "source=in_memory_reuse") {
		t.Errorf("missing reuse source:\n%s", out)
	}
	if !strings.Contains(out, "verify selected files") {
		t.Errorf("reused graph must carry the verify reminder:\n%s", out)
	}
}

func TestIndexStatusBanner_ZeroValueRendersUnknownNeverFabricates(t *testing.T) {
	g := bannerTestGraph(rmtypes.RepoMapIndexStatus{}, 0)
	out := prependRepoMapIndexStatusBanner(g, "BODY")
	if !strings.Contains(out, "source=unknown") {
		t.Errorf("zero-value status must render unknown:\n%s", out)
	}
	if strings.Contains(out, "source=full_scan") {
		t.Errorf("zero-value status fabricated full_scan:\n%s", out)
	}
	if !strings.Contains(out, "verify selected files") {
		t.Errorf("unknown freshness must carry the verify reminder:\n%s", out)
	}
}

func TestIndexStatusBanner_BannerHeightCapped(t *testing.T) {
	// Worst case: rebuilt + fallback + reuse — still ≤5 lines + blank.
	g := bannerTestGraph(rmtypes.RepoMapIndexStatus{
		Source:         rmtypes.IndexSourceFullScan,
		RebuildReasons: []string{"extractor_version_mismatch"},
	}, 5)
	out := prependRepoMapIndexStatusBanner(g, "BODY")
	banner := strings.SplitN(out, "\n\nBODY", 2)[0]
	if lines := strings.Count(banner, "\n") + 1; lines > 5 {
		t.Errorf("banner is %d lines, cap is 5:\n%s", lines, banner)
	}
}

func TestIndexStatusBanner_CloneStampsReuse(t *testing.T) {
	base := bannerTestGraph(rmtypes.RepoMapIndexStatus{
		Source:    rmtypes.IndexSourceCacheHit,
		Freshness: rmtypes.IndexFreshnessFresh,
	}, 0)
	clone := cloneGraphForRanking(base)
	clone.Metadata.IndexStatus.Source = rmtypes.IndexSourceInMemoryReuse
	clone.Metadata.IndexStatus.Freshness = rmtypes.IndexFreshnessReused
	// The resident must keep its own status: Metadata is value-copied.
	if base.Metadata.IndexStatus.Source != rmtypes.IndexSourceCacheHit {
		t.Errorf("clone stamp mutated the resident graph's status: %+v", base.Metadata.IndexStatus)
	}
	if clone.Metadata.IndexStatus.Source != rmtypes.IndexSourceInMemoryReuse {
		t.Errorf("clone not stamped: %+v", clone.Metadata.IndexStatus)
	}
}
