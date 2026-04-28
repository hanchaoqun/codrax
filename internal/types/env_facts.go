package types

import "time"

// EnvFacts is the structured snapshot of the operator's runtime
// environment that env_recommend uses to drive diagnosis +
// recommendation. Built once per Run by internal/env/probe and
// stashed on the BusContext for tools (and the /env REPL command)
// to read. Read-only after construction; the producer is the only
// writer.
//
// Fields default to zero / empty when probing fails — every
// downstream consumer must nil-check before using a sub-slice or
// map. The probe layer never errors fatally; partial facts are
// always better than no facts.
type EnvFacts struct {
	OS              string                    `json:"os"`         // linux / darwin / windows
	Arch            string                    `json:"arch"`       // amd64 / arm64
	OSFamily        string                    `json:"os_family"`  // debian / rhel / arch / alpine / darwin / windows / freebsd / unknown
	OSVersion       string                    `json:"os_version"` // "ubuntu 22.04" / "macos 14.3"
	Shell           string                    `json:"shell"`      // bash / zsh / fish / pwsh / cmd / unknown
	PathTools       map[string]string         `json:"path_tools,omitempty"`    // bin name → resolved path
	PkgManagers     map[string]PkgManagerInfo `json:"pkg_managers,omitempty"`  // apt / dnf / brew / pip / etc
	ProjectFiles    map[string]string         `json:"project_files,omitempty"` // pyproject.toml / package.json → abs path
	Pythons         []PythonInterp            `json:"pythons,omitempty"`
	Nodes           []NodeInterp              `json:"nodes,omitempty"`
	Rusts           []RustToolchain           `json:"rusts,omitempty"`
	Javas           []JavaToolchain           `json:"javas,omitempty"`
	Rubies          []RubyToolchain           `json:"rubies,omitempty"`
	Network         NetworkStatus             `json:"network,omitempty"`
	GitRepoState    string                    `json:"git_repo_state,omitempty"` // ready / not_initialized / no_commits / git_missing / unknown
	InsideContainer bool                      `json:"inside_container,omitempty"`
	ProbedAt        time.Time                 `json:"probed_at"`
	ProbeDurationMs int64                     `json:"probe_duration_ms,omitempty"`
}

// PkgManagerInfo captures one package manager's presence and version
// (best-effort — Version may be empty when parsing failed).
type PkgManagerInfo struct {
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
}

// PythonInterp describes one Python interpreter discovered on the
// system. Origin distinguishes system Python from venv / pyenv /
// conda so Recommender can route differently. HasPytest /
// HasJsonReport are the two pytest-related fast checks the probe
// runs against the interpreter's site-packages.
type PythonInterp struct {
	Path             string `json:"path"`
	Version          string `json:"version,omitempty"`
	Origin           string `json:"origin"` // system / venv / pyenv / conda
	HasPytest        bool   `json:"has_pytest,omitempty"`
	HasJsonReport    bool   `json:"has_json_report,omitempty"`
	SitePackagesPath string `json:"site_packages_path,omitempty"`
}

// NodeInterp pairs a node binary with its accompanying package
// manager(s). A single node install can have npm + pnpm + yarn +
// bun all installed — the slice captures whatever the probe found.
type NodeInterp struct {
	Path        string   `json:"path"`
	Version     string   `json:"version,omitempty"`
	PkgManagers []string `json:"pkg_managers,omitempty"` // [npm pnpm yarn bun]
}

// RustToolchain captures rustc + cargo. A rustup-managed install
// might have multiple toolchains; we pick the active one.
type RustToolchain struct {
	RustcPath string `json:"rustc_path"`
	CargoPath string `json:"cargo_path,omitempty"`
	Version   string `json:"version,omitempty"`
	Origin    string `json:"origin"` // rustup / system / unknown
}

// JavaToolchain pairs the JDK with build-tool wrappers. Maven and
// Gradle are independent — the probe records whichever is present.
type JavaToolchain struct {
	JavacPath        string `json:"javac_path"`
	JavaVersion      string `json:"java_version,omitempty"`
	MavenPath        string `json:"maven_path,omitempty"`
	GradlePath       string `json:"gradle_path,omitempty"`
	HasGradleWrapper bool   `json:"has_gradle_wrapper,omitempty"`
}

// RubyToolchain captures the ruby + bundler pair the runner needs.
type RubyToolchain struct {
	RubyPath    string `json:"ruby_path"`
	Version     string `json:"version,omitempty"`
	BundlerPath string `json:"bundler_path,omitempty"`
}

// NetworkStatus is the result of the probe's reachability check.
// Reachable=false in offline / firewalled environments; the
// Recommender uses it to demote NeedsNetwork=true candidates.
type NetworkStatus struct {
	Reachable bool      `json:"reachable"`
	ProxySet  bool      `json:"proxy_set,omitempty"` // HTTP_PROXY / HTTPS_PROXY env detected
	ProbedAt  time.Time `json:"probed_at,omitempty"`
}

// HasPython is the convenience accessor most callers use — at least
// one Python interpreter exists on PATH or in a project venv.
func (e *EnvFacts) HasPython() bool {
	return e != nil && len(e.Pythons) > 0
}

// HasPip reports whether any Python interpreter has pip available
// (either as bundled or as a sibling pip3 binary). Used by Python
// Recommender to choose between venv-install and user-install.
func (e *EnvFacts) HasPip() bool {
	if e == nil {
		return false
	}
	for k := range e.PkgManagers {
		if k == "pip" || k == "pip3" {
			return true
		}
	}
	return false
}

// HasNode reports whether a usable node binary was discovered.
func (e *EnvFacts) HasNode() bool {
	return e != nil && len(e.Nodes) > 0
}

// HasPkgManager is the generic accessor — does the operator have
// the named package manager installed? Used by the Recommender
// when deciding which install command to surface (apt vs brew vs
// dnf, etc.). Returns false safely on nil receiver.
func (e *EnvFacts) HasPkgManager(name string) bool {
	if e == nil || e.PkgManagers == nil {
		return false
	}
	_, ok := e.PkgManagers[name]
	return ok
}

// HasProjectFile is the convenience accessor for "is there a
// pyproject.toml / package.json / Cargo.toml / etc. somewhere in
// the repo?" Used by Recommender to pick install strategies.
func (e *EnvFacts) HasProjectFile(name string) bool {
	if e == nil || e.ProjectFiles == nil {
		return false
	}
	_, ok := e.ProjectFiles[name]
	return ok
}

// Diagnosis is the structured output of the diag layer. Layer-1
// detectors emit one of these, the Recommender consumes it. Empty
// Source / Confidence allowed — used as a sentinel by the LLM
// fallback path when no deterministic match was found.
type Diagnosis struct {
	Kind       DiagKind `json:"kind"`
	Runner     string   `json:"runner,omitempty"`
	Tool       string   `json:"tool,omitempty"`
	Source     string   `json:"source,omitempty"`
	Excerpt    string   `json:"excerpt,omitempty"`
	Confidence float32  `json:"confidence,omitempty"`
}

// DiagKind enumerates the eight diagnosis classes env_recommend
// covers. New kinds added later should keep the design's
// constraint: each kind maps to a small, independent set of
// install strategies.
type DiagKind string

const (
	DiagRunnerMissing         DiagKind = "runner_missing"
	DiagDepsMissing           DiagKind = "deps_missing"
	DiagBuildToolchainMissing DiagKind = "toolchain_missing"
	DiagSystemLibMissing      DiagKind = "system_lib_missing"
	DiagConfigMissing         DiagKind = "config_missing"
	DiagGitNotInstalled       DiagKind = "git_not_installed"
	DiagGitRepoNotInitialized DiagKind = "git_not_initialized"
	DiagGitRepoNoCommits      DiagKind = "git_no_commits"
	DiagUnknown               DiagKind = "unknown" // LLM-fallback sentinel
)

// InstallStrategy classifies a Recommendation by safety / scope.
// The Recommender ranks candidates by this enum (lowest int first
// = safest), and the Renderer prefixes risky strategies with extra
// warning text so operators see the trade-off before they execute.
type InstallStrategy int

const (
	StrategyVenvInstall        InstallStrategy = iota // project-isolated; safest
	StrategyProjectInstall                            // npm install / cargo fetch / bundle install --path
	StrategyUserInstall                               // pip --user / gem install --user-install
	StrategyToolchainBootstrap                        // rustup-init / nvm / pyenv
	StrategyGlobalInstall                             // sudo apt / brew / npm install -g — gated by config
	StrategyDocsLink                                  // no good command, only a link
)

// Recommendation is one install candidate the Recommender produces
// for a given Diagnosis. Multiple recommendations may apply to one
// diagnosis (e.g. venv-install + user-install for missing pytest);
// they are sorted by Priority (lower wins) before the Renderer
// emits the top ~3.
type Recommendation struct {
	Strategy     InstallStrategy `json:"strategy"`
	Command      string          `json:"command"`
	Why          string          `json:"why,omitempty"`
	NeedsNetwork bool            `json:"needs_network,omitempty"`
	NeedsSudo    bool            `json:"needs_sudo,omitempty"`
	EstimatedSec int             `json:"estimated_sec,omitempty"`
	Caveats      []string        `json:"caveats,omitempty"`
	Priority     int             `json:"priority,omitempty"`
	DiagRef      Diagnosis       `json:"diag_ref,omitempty"`
}

// EnvRecommendSettings carries the codrax.yaml knobs governing the
// env_recommend subsystem. All fields have sensible code defaults
// in DefaultEnvRecommendSettings; ResolvedEnvRecommendSettings
// hydrates zero values back to defaults for callers that built a
// partial struct.
type EnvRecommendSettings struct {
	// Enabled is the master switch. false → recommender layer is
	// bypassed entirely and run_tests etc. fall back to the
	// pre-env_recommend hardcoded hint strings (R6 red line).
	Enabled bool `yaml:"enabled"`

	// LLMEnabled gates the Stage-3 LLM fallback (R1 red line).
	// false → only the deterministic table is consulted; missing
	// matches surface a generic DocsLink rather than calling
	// env_recommender LLM.
	LLMEnabled bool `yaml:"llm_enabled"`

	// LLMTimeoutSec caps the Stage-3 LLM call. Hitting this drops
	// to deterministic-only result (failing safe per R5).
	LLMTimeoutSec int `yaml:"llm_timeout_sec"`

	// RecommendGlobalInstall gates whether StrategyGlobalInstall
	// candidates are shown at all (R8 red line). Default false —
	// sudo / brew / npm install -g require explicit operator opt-in.
	RecommendGlobalInstall bool `yaml:"recommend_global_install"`

	// ProbeNetwork enables the reachability probe in Layer 2.
	// Disabling saves ~200ms startup.
	ProbeNetwork bool `yaml:"probe_network"`

	// CacheTTLDays is the lifetime of a disk-cache entry before
	// it's re-queried. 90 days mirrors common stable-package-name
	// horizons.
	CacheTTLDays int `yaml:"cache_ttl_days"`
}

// DefaultEnvRecommendSettings returns the production defaults.
// New keys added here also need a yaml line in
// internal/config/runtime.go and a documented entry in
// codrax.yaml.example.
func DefaultEnvRecommendSettings() EnvRecommendSettings {
	return EnvRecommendSettings{
		Enabled:                true,
		LLMEnabled:             true,
		LLMTimeoutSec:          5,
		RecommendGlobalInstall: false,
		ProbeNetwork:           true,
		CacheTTLDays:           90,
	}
}

// ResolvedEnvRecommendSettings fills zero fields from defaults.
// Used by cmd/root.go after merging yaml overrides so downstream
// callers always see a fully-populated struct.
func ResolvedEnvRecommendSettings(s EnvRecommendSettings) EnvRecommendSettings {
	d := DefaultEnvRecommendSettings()
	// Bool fields don't have a "zero means default" interpretation
	// (zero == false is also a meaningful explicit value), so the
	// resolver only patches them when the original struct was the
	// strict zero value of every field — i.e. no yaml customisation
	// at all. yaml-loading code in cmd/root.go is responsible for
	// passing pointer-typed yaml values so true/false intent is
	// distinguishable from "absent".
	if s.LLMTimeoutSec == 0 {
		s.LLMTimeoutSec = d.LLMTimeoutSec
	}
	if s.CacheTTLDays == 0 {
		s.CacheTTLDays = d.CacheTTLDays
	}
	return s
}
