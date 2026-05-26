package types

// RepoMapScanMode classifies why a repository map build is touching
// the filesystem. The value is telemetry only: renderers and logs use
// it for user-facing wording, while cache correctness remains decided
// by the repomap index layer.
type RepoMapScanMode string

const (
	RepoMapScanFull        RepoMapScanMode = "full"
	RepoMapScanFullRescan  RepoMapScanMode = "full_rescan"
	RepoMapScanIncremental RepoMapScanMode = "incremental"
	RepoMapScanCacheHit    RepoMapScanMode = "cache_hit"
)

// RepoMapScanPhase names the local CPU/IO phase currently responsible
// for repo_map latency. It keeps UI wording honest after parsing
// completes but graph construction or cache IO is still running.
type RepoMapScanPhase string

const (
	RepoMapScanPhaseFileScan   RepoMapScanPhase = "file_scan"
	RepoMapScanPhaseChangeScan RepoMapScanPhase = "change_scan"
	RepoMapScanPhaseParse      RepoMapScanPhase = "parse"
	RepoMapScanPhaseBuildGraph RepoMapScanPhase = "build_graph"
	RepoMapScanPhaseRank       RepoMapScanPhase = "rank"
	RepoMapScanPhaseCacheLoad  RepoMapScanPhase = "cache_load"
	RepoMapScanPhaseCacheWrite RepoMapScanPhase = "cache_write"
	RepoMapScanPhaseWait       RepoMapScanPhase = "wait"
)

// RepoMapScanEvent is the typed progress payload emitted by the
// single-repo repomap builder while it is parsing a large repository.
// It deliberately carries counts and booleans, not rendered strings,
// so UI/log wording stays localized at the render/orchestrator layer.
type RepoMapScanEvent struct {
	RepoRoot string
	// DisplayName is an optional UI label supplied by the app layer,
	// for example a multi-repo RootRel. Empty means renderers derive a
	// short label from RepoRoot.
	DisplayName string
	// SubRepoRootRel is set when RepoRoot belongs to a discovered
	// multi-repo child. Message helpers use it to say "sub-repo index"
	// instead of the generic "repo index".
	SubRepoRootRel string
	Mode           RepoMapScanMode
	Phase          RepoMapScanPhase

	Started  bool
	Progress bool
	Finished bool
	OK       bool

	TotalFiles     int
	ParseableFiles int
	ParsedFiles    int
	ChangedFiles   int
	CurrentFile    string
	BytesWritten   int64
	BytesTotal     int64

	ElapsedMs int64
	Error     string
}
