package dataflow

import (
	"context"
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// FlowNodeKind is the normalized node type in the cross-language IR.
type FlowNodeKind string

const (
	NodeSymbol      FlowNodeKind = "symbol"
	NodeParam       FlowNodeKind = "param"
	NodeLocal       FlowNodeKind = "local"
	NodeField       FlowNodeKind = "field"
	NodeLiteral     FlowNodeKind = "literal"
	NodeCallsite    FlowNodeKind = "callsite"
	NodeReturn      FlowNodeKind = "return"
	NodeConfigKey   FlowNodeKind = "config_key"
	NodeConfigValue FlowNodeKind = "config_value"
	NodeEnvKey      FlowNodeKind = "env_key"
)

// FlowEdgeKind is the normalized edge type in the cross-language IR.
type FlowEdgeKind string

const (
	EdgeDefines     FlowEdgeKind = "defines"
	EdgeAssigns     FlowEdgeKind = "assigns"
	EdgeLoadsField  FlowEdgeKind = "loads_field"
	EdgeStoresField FlowEdgeKind = "stores_field"
	EdgePassesArg   FlowEdgeKind = "passes_arg"
	EdgeReturns     FlowEdgeKind = "returns"
	EdgeCalls       FlowEdgeKind = "calls"
	EdgeConstructs  FlowEdgeKind = "constructs"
	EdgeAliases     FlowEdgeKind = "aliases"
	EdgeGuards      FlowEdgeKind = "guards"
	EdgeReadsConfig FlowEdgeKind = "reads_config"
	EdgeBinds       FlowEdgeKind = "binds"
	EdgeImports     FlowEdgeKind = "imports"
)

// FlowNode is the generic IR node used by all language lowerers.
type FlowNode struct {
	ID        string       `json:"id"`
	Kind      FlowNodeKind `json:"kind"`
	Name      string       `json:"name"`
	Source    string       `json:"source,omitempty"`
	LineStart int          `json:"line_start,omitempty"`
	LineEnd   int          `json:"line_end,omitempty"`
}

// FlowEdge is the generic IR edge used by all language lowerers.
type FlowEdge struct {
	From      string       `json:"from"`
	To        string       `json:"to"`
	Kind      FlowEdgeKind `json:"kind"`
	Condition string       `json:"condition,omitempty"`
	Source    string       `json:"source,omitempty"`
	Line      int          `json:"line,omitempty"`
}

// GuardExpr is a preserved conditional guard from the source code.
type GuardExpr struct {
	Expr      string `json:"expr"`
	Source    string `json:"source,omitempty"`
	LineStart int    `json:"line_start,omitempty"`
	LineEnd   int    `json:"line_end,omitempty"`
}

// FunctionSummary is the compact interprocedural summary used by the
// dataflow worklist.
type FunctionSummary struct {
	SymbolKey        string               `json:"symbol_key"`
	SymbolName       string               `json:"symbol_name"`
	File             string               `json:"file"`
	Language         string               `json:"language"`
	LineStart        int                  `json:"line_start"`
	LineEnd          int                  `json:"line_end"`
	Calls            []string             `json:"calls,omitempty"`
	Returns          []string             `json:"returns,omitempty"`
	Literals         []string             `json:"literals,omitempty"`
	ReadFields       []string             `json:"read_fields,omitempty"`
	WrittenFields    []string             `json:"written_fields,omitempty"`
	ReadConfigKeys   []string             `json:"read_config_keys,omitempty"`
	ConstructedTypes []string             `json:"constructed_types,omitempty"`
	AliasTargets     []string             `json:"alias_targets,omitempty"`
	Guards           []GuardExpr          `json:"guards,omitempty"`
	HasUnknownEffect bool                 `json:"has_unknown_effect,omitempty"`
	UnknownReason    string               `json:"unknown_reason,omitempty"`
	CallSites        map[string]int       `json:"call_sites,omitempty"`
	ProducerEvidence []types.EvidenceItem `json:"producer_evidence,omitempty"`
}

// FlowFinding is the internal verbose form before digesting for stage
// transport.
type FlowFinding struct {
	Path              []string
	Conditions        []string
	Sources           []string
	Sinks             []string
	Hops              []string
	EvidenceIDs       []string
	UnsupportedReason string
	Confidence        float64
}

// LoweredFile is the per-file lowering result cached by the engine.
type LoweredFile struct {
	File      string               `json:"file"`
	Language  string               `json:"language"`
	Hash      string               `json:"hash"`
	NodeCount int                  `json:"node_count"`
	Summaries []FunctionSummary    `json:"summaries,omitempty"`
	Evidence  []types.EvidenceItem `json:"evidence,omitempty"`
}

// Options controls the bounded scope of the analysis.
type Options struct {
	Context         context.Context
	RepoRoot        string
	Question        string
	Scope           []string
	CandidateFiles  []string
	WorkDir         string
	MaxFiles        int
	MaxIterations   int
	MaxNodesPerFunc int
	// SkipFindings disables the multi-hop buildFindings pass. Lowering
	// and per-file evidence are still produced. Used for "Lookup"-intent
	// questions where single-hop evidence is enough and the cross-file
	// propagation pass would be wasted compute.
	SkipFindings bool
	// EntityBias (T2.3) is a list of question-relevant entity names
	// (typically derived from ERM). When non-empty, selectCandidateFiles
	// scores each candidate by the number of biased entities that appear
	// in the file path OR in the file's symbol names, then sorts by
	// score descending before truncating to MaxFiles. Files with no
	// match still survive — the bias only re-ranks; it does not filter.
	//
	// When EntityBias is non-empty, it is also consulted at symbol
	// granularity inside LowerFile: symbols whose name AND whose file
	// path both miss every bias entity are skipped — their per-line
	// evidence (guards, returns, calls, field reads/writes) is not
	// produced. This is the producer-level relevance gate that prevents
	// large files such as explorer.go (8k LOC, hundreds of symbols)
	// from flooding the evidence stream when only a handful of symbols
	// are on-topic. When EntityBias is empty the gate is a no-op so
	// pre-bias callers keep legacy semantics.
	EntityBias []string

	// MaxItemsPerFile caps the number of ProducerEvidence items a
	// single LowerFile call may emit. Deterministic lowerers like
	// genericLowerer produce one EvidenceItem per guard / return
	// literal / config key / field read/write / call site; a single
	// long file can therefore emit thousands of items. The cap
	// guarantees no individual file dominates downstream Primary
	// Evidence selection and caps the total bus.Mutable.StructuredEvidence
	// size regardless of how skewed the EntityBias gate's pass rate is
	// across candidate files. Zero disables the cap (legacy behaviour).
	MaxItemsPerFile int
}

// Result is the compact output consumed by explorer/sub-explorer.
type Result struct {
	Evidence []types.EvidenceItem
	Findings []types.FlowFindingDigest
}

// Lowerer lowers a parsed file into the cross-language IR/summaries.
type Lowerer interface {
	Language() string
	LowerFile(repoRoot string, file *repomap.FileInfo, source []string, opts Options) LoweredFile
}

type summaryCacheEntry struct {
	Key     string          `json:"key"`
	Summary FunctionSummary `json:"summary"`
}

func defaultOptions(opts Options) Options {
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 40
	}
	if opts.MaxIterations <= 0 {
		opts.MaxIterations = 6
	}
	if opts.MaxNodesPerFunc <= 0 {
		opts.MaxNodesPerFunc = 400
	}
	if opts.MaxItemsPerFile < 0 {
		opts.MaxItemsPerFile = 0
	}
	return opts
}

func marshalJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}
