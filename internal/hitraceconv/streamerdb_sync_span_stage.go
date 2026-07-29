package hitraceconv

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const (
	defaultTraceDBSyncSpanResidentBytes       int64 = 8 << 20
	defaultTraceDBSyncSpanMaxRecords          int64 = 4_000_000
	defaultTraceDBSyncSpanMaxTempBytes        int64 = 4 << 30
	defaultTraceDBSyncSpanMaxActiveDepth            = 131_072
	defaultTraceDBSyncSpanMaxActiveBytes      int64 = 64 << 20
	defaultTraceDBSyncSpanMaxAuditComparisons int64 = 100_000_000
	defaultTraceDBSyncSpanSQLiteCacheKiB            = 16 << 10
	traceDBSyncSpanSQLitePageBytes                  = 4096
	traceDBSyncSpanSQLiteMinimumPages               = 16
	traceDBSyncSpanSQLiteWriteReservePages          = 16
	traceDBSyncSpanSQLiteForcedReservePages         = 8
	traceDBSyncSpanLaneOrderKeyBytes                = 47
)

const (
	traceDBSyncSpanStageBudgetRecordCap       = "record_cap"
	traceDBSyncSpanStageBudgetSQLitePageCap   = "sqlite_page_cap"
	traceDBSyncSpanStageBudgetTempByteCap     = "temp_byte_cap"
	traceDBSyncSpanStageBudgetActiveDepthCap  = "active_depth_cap"
	traceDBSyncSpanStageBudgetActiveByteCap   = "active_byte_cap"
	traceDBSyncSpanStageBudgetAuditCompareCap = "audit_comparison_cap"
	traceDBSyncSpanStageBudgetSequenceCap     = "endpoint_sequence_cap"
)

type traceDBSyncSpanStageOptions struct {
	TempRoot            string
	ResidentBytes       int64
	MaxRecords          int64
	MaxTempBytes        int64
	MaxActiveDepth      int
	MaxActiveBytes      int64
	MaxAuditComparisons int64
	SQLiteCacheKiB      int
}

func normalizedTraceDBSyncSpanStageOptions(options traceDBSyncSpanStageOptions) traceDBSyncSpanStageOptions {
	if options.ResidentBytes <= 0 {
		options.ResidentBytes = defaultTraceDBSyncSpanResidentBytes
	}
	if options.MaxRecords <= 0 {
		options.MaxRecords = defaultTraceDBSyncSpanMaxRecords
	}
	if options.MaxTempBytes <= 0 {
		options.MaxTempBytes = defaultTraceDBSyncSpanMaxTempBytes
	}
	if options.MaxActiveDepth <= 0 {
		options.MaxActiveDepth = defaultTraceDBSyncSpanMaxActiveDepth
	}
	if options.MaxActiveBytes <= 0 {
		options.MaxActiveBytes = defaultTraceDBSyncSpanMaxActiveBytes
	}
	if options.MaxAuditComparisons <= 0 {
		options.MaxAuditComparisons = defaultTraceDBSyncSpanMaxAuditComparisons
	}
	if options.SQLiteCacheKiB <= 0 {
		options.SQLiteCacheKiB = defaultTraceDBSyncSpanSQLiteCacheKiB
	}
	return options
}

type traceDBSyncSpanForcedReason uint8

const (
	traceDBSyncSpanForcedNone            traceDBSyncSpanForcedReason = 0
	traceDBSyncSpanForcedDuplicate       traceDBSyncSpanForcedReason = 1 << 0
	traceDBSyncSpanForcedCallstackPoison traceDBSyncSpanForcedReason = 1 << 1
	traceDBSyncSpanForcedSyscallPoison   traceDBSyncSpanForcedReason = 1 << 2
	traceDBSyncSpanForcedPoison                                      = traceDBSyncSpanForcedCallstackPoison | traceDBSyncSpanForcedSyscallPoison
	traceDBSyncSpanForcedAll                                         = traceDBSyncSpanForcedDuplicate | traceDBSyncSpanForcedPoison
)

type traceDBSyncSpanStageStats struct {
	PeakResidentCandidates int
	PeakResidentBytes      int64
	PeakActiveDepth        int
	PeakActiveBytes        int64
	AuditComparisons       int64
	ExternalArtifacts      int
	PeakTempBytes          int64
	Backend                string
	LanePlanVerified       bool
}

type traceDBSyncSpanStageBudgetError struct {
	Reason string
}

func (err *traceDBSyncSpanStageBudgetError) Error() string {
	return "trace DB sync span stage budget exceeded: " + err.Reason
}

func traceDBSyncSpanStageBudgetReason(err error) (string, bool) {
	var budget *traceDBSyncSpanStageBudgetError
	if !errors.As(err, &budget) {
		return "", false
	}
	return budget.Reason, true
}

type traceDBSyncSpanStage struct {
	options                       traceDBSyncSpanStageOptions
	workspace                     string
	dbPath                        string
	db                            *sql.DB
	conn                          *sql.Conn
	tx                            *sql.Tx
	insertCandidate               *sql.Stmt
	insertIdentity                *sql.Stmt
	lookupIdentity                *sql.Stmt
	lookupSemantic                *sql.Stmt
	lookupInterval                *sql.Stmt
	lookupSemanticLocallyAdmitted *sql.Stmt
	lookupIntervalLocallyAdmitted *sql.Stmt
	censusIntervalLocallyAdmitted *sql.Stmt
	supersedeCPUUnavailable       *sql.Stmt
	selectCPUKnownCallstack       *sql.Stmt
	supersedeCPUKnownCallstack    *sql.Stmt
	upsertForced                  *sql.Stmt
	insertFence                   *sql.Stmt

	memoryCandidates    []traceDBSyncSpanStagedCandidate
	memoryFences        []traceDBSyncSpanStagedFence
	memoryIdentityFirst map[traceDBSyncSpanIdentity]int64
	memoryForced        map[int64]traceDBSyncSpanForcedReason
	residentBytes       int64
	records             int64
	external            bool
	sealed              bool
	closed              bool
	cleanupErr          error
	budgetReason        string
	maxSQLitePages      int64
	tempAccountedBytes  int64
	stats               traceDBSyncSpanStageStats
}

type traceDBSyncSpanStagedCandidate struct {
	Ordinal   int64
	Candidate traceDBSyncSpanCandidate
}

type traceDBSyncSpanStagedFence struct {
	Ordinal int64
	Fence   traceDBSyncSpanLaneFence
}

func newTraceDBSyncSpanStage(ctx context.Context, options traceDBSyncSpanStageOptions) (*traceDBSyncSpanStage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	options = normalizedTraceDBSyncSpanStageOptions(options)
	if options.MaxTempBytes < traceDBSyncSpanSQLitePageBytes || options.MaxRecords > math.MaxInt/2 {
		return nil, &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_options"}
	}
	workspace, err := os.MkdirTemp(options.TempRoot, "codrax-tracedb-sync-*")
	if err != nil {
		return nil, fmt.Errorf("create sync span stage workspace: %w", err)
	}
	if err := os.Chmod(workspace, 0o700); err != nil {
		removeErr := os.RemoveAll(workspace)
		return nil, errors.Join(fmt.Errorf("secure sync span stage workspace: %w", err), removeErr)
	}
	stage := &traceDBSyncSpanStage{
		options:             options,
		workspace:           workspace,
		memoryIdentityFirst: map[traceDBSyncSpanIdentity]int64{},
		memoryForced:        map[int64]traceDBSyncSpanForcedReason{},
	}
	stage.stats.Backend = "memory"
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, stage.cleanup())
	}
	return stage, nil
}

func (stage *traceDBSyncSpanStage) addCandidate(ctx context.Context, candidate traceDBSyncSpanCandidate) error {
	if err := stage.requireOpen(ctx); err != nil {
		return err
	}
	if stage.budgetReason != "" {
		return nil
	}
	ordinal, err := stage.admitRecord()
	if err != nil {
		return err
	}
	if stage.external {
		return stage.insertSQLiteCandidate(ctx, ordinal, candidate)
	}
	delta := traceDBSyncSpanCandidateResidentBytes(candidate)
	identity := traceDBSyncSpanIdentity{Producer: candidate.Producer, StableKind: candidate.StableKind, StableID: candidate.StableID}
	firstTID, duplicate := stage.memoryIdentityFirst[identity]
	if !duplicate {
		delta += 64
	} else {
		if stage.memoryForced[firstTID]&traceDBSyncSpanForcedDuplicate == 0 {
			delta += 32
		}
		if stage.memoryForced[candidate.HeaderTID]&traceDBSyncSpanForcedDuplicate == 0 && candidate.HeaderTID != firstTID {
			delta += 32
		}
	}
	if stage.residentBytes > stage.options.ResidentBytes-delta {
		if err := stage.promoteToSQLite(ctx); err != nil {
			return err
		}
		if stage.budgetReason != "" {
			return nil
		}
		return stage.insertSQLiteCandidate(ctx, ordinal, candidate)
	}
	stage.memoryCandidates = append(stage.memoryCandidates, traceDBSyncSpanStagedCandidate{
		Ordinal: ordinal, Candidate: candidate,
	})
	stage.residentBytes += traceDBSyncSpanCandidateResidentBytes(candidate)
	if duplicate {
		stage.addMemoryForced(firstTID, traceDBSyncSpanForcedDuplicate)
		stage.addMemoryForced(candidate.HeaderTID, traceDBSyncSpanForcedDuplicate)
	} else {
		stage.memoryIdentityFirst[identity] = candidate.HeaderTID
		stage.residentBytes += 64
	}
	stage.observeResident()
	return nil
}

func (stage *traceDBSyncSpanStage) addPoison(ctx context.Context, poison traceDBSyncSpanLanePoison) error {
	if err := stage.requireOpen(ctx); err != nil {
		return err
	}
	if stage.budgetReason != "" {
		return nil
	}
	if _, err := stage.admitRecord(); err != nil {
		return err
	}
	reason := traceDBSyncSpanProducerPoisonReason(poison.Producer)
	if reason == traceDBSyncSpanForcedNone {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_poison_producer"}
	}
	if stage.external {
		return stage.upsertSQLiteForced(ctx, poison.HeaderTID, reason)
	}
	delta := int64(0)
	if stage.memoryForced[poison.HeaderTID] == traceDBSyncSpanForcedNone {
		delta = 32
	}
	if stage.residentBytes > stage.options.ResidentBytes-delta {
		if err := stage.promoteToSQLite(ctx); err != nil {
			return err
		}
		if stage.budgetReason != "" {
			return nil
		}
		return stage.upsertSQLiteForced(ctx, poison.HeaderTID, reason)
	}
	stage.addMemoryForced(poison.HeaderTID, reason)
	stage.observeResident()
	return nil
}

func (stage *traceDBSyncSpanStage) addFence(ctx context.Context, fence traceDBSyncSpanLaneFence) error {
	if err := stage.requireOpen(ctx); err != nil {
		return err
	}
	if stage.budgetReason != "" {
		return nil
	}
	ordinal, err := stage.admitRecord()
	if err != nil {
		return err
	}
	if stage.external {
		return stage.insertSQLiteFence(ctx, ordinal, fence)
	}
	// Covers the typed record plus worst-case slice growth where the old and
	// newly allocated backing arrays are simultaneously resident.
	const residentBytes int64 = 256
	if stage.residentBytes > stage.options.ResidentBytes-residentBytes {
		if err := stage.promoteToSQLite(ctx); err != nil {
			return err
		}
		if stage.budgetReason != "" {
			return nil
		}
		return stage.insertSQLiteFence(ctx, ordinal, fence)
	}
	stage.memoryFences = append(stage.memoryFences, traceDBSyncSpanStagedFence{
		Ordinal: ordinal,
		Fence:   fence,
	})
	stage.residentBytes += residentBytes
	stage.observeResident()
	return nil
}

func traceDBSyncSpanProducerPoisonReason(producer traceDBSyncSpanProducer) traceDBSyncSpanForcedReason {
	switch producer {
	case traceDBSyncSpanProducerCallstack:
		return traceDBSyncSpanForcedCallstackPoison
	case traceDBSyncSpanProducerSyscall:
		return traceDBSyncSpanForcedSyscallPoison
	default:
		return traceDBSyncSpanForcedNone
	}
}

func traceDBSyncSpanProducerPoisoned(mask traceDBSyncSpanForcedReason, producer traceDBSyncSpanProducer) bool {
	return producer == traceDBSyncSpanProducerCallstack &&
		mask&traceDBSyncSpanForcedCallstackPoison != 0
}

func (stage *traceDBSyncSpanStage) requireOpen(ctx context.Context) error {
	if stage == nil || stage.closed || stage.sealed {
		return &traceDBOutputInvariantError{Reason: "sync_span_stage_not_open"}
	}
	if ctx == nil {
		return &traceDBOutputInvariantError{Reason: "missing_sync_span_stage_context"}
	}
	return ctx.Err()
}

func (stage *traceDBSyncSpanStage) admitRecord() (int64, error) {
	if stage.records >= stage.options.MaxRecords {
		return 0, stage.failBudget(traceDBSyncSpanStageBudgetRecordCap)
	}
	stage.records++
	return stage.records, nil
}

func traceDBSyncSpanCandidateResidentBytes(candidate traceDBSyncSpanCandidate) int64 {
	return 256 + int64(len(candidate.Task)) + int64(len(candidate.Name)) +
		int64(len(candidate.StartMarkerBody)) + int64(len(candidate.EndMarkerBody)) +
		traceDBSyncSpanLaneOrderKeyBytes
}

func (stage *traceDBSyncSpanStage) addMemoryForced(tid int64, reason traceDBSyncSpanForcedReason) {
	previous := stage.memoryForced[tid]
	if previous == traceDBSyncSpanForcedNone {
		stage.residentBytes += 32
	}
	stage.memoryForced[tid] = previous | reason
}

func (stage *traceDBSyncSpanStage) observeResident() {
	if len(stage.memoryCandidates) > stage.stats.PeakResidentCandidates {
		stage.stats.PeakResidentCandidates = len(stage.memoryCandidates)
	}
	if stage.residentBytes > stage.stats.PeakResidentBytes {
		stage.stats.PeakResidentBytes = stage.residentBytes
	}
}

func (stage *traceDBSyncSpanStage) promoteToSQLite(ctx context.Context) error {
	if stage.external || stage.budgetReason != "" {
		return nil
	}
	if err := applyTraceDBSQLiteHardHeapLimit(); err != nil {
		return err
	}
	stage.maxSQLitePages = stage.options.MaxTempBytes / traceDBSyncSpanSQLitePageBytes
	if stage.maxSQLitePages < traceDBSyncSpanSQLiteMinimumPages {
		return stage.failBudget(traceDBSyncSpanStageBudgetSQLitePageCap)
	}
	stage.dbPath = filepath.Join(stage.workspace, "sync-span-stage.sqlite")
	db, err := sql.Open("sqlite", stage.dbPath)
	if err != nil {
		return fmt.Errorf("open private sync span stage: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	stage.db = db
	conn, err := db.Conn(ctx)
	if err != nil {
		return stage.handleSQLiteWriteError(ctx, fmt.Errorf("acquire private sync span stage connection: %w", err))
	}
	stage.conn = conn
	if err := conn.PingContext(ctx); err != nil {
		return stage.handleSQLiteWriteError(ctx, fmt.Errorf("create private sync span stage: %w", err))
	}
	if err := os.Chmod(stage.dbPath, 0o600); err != nil {
		return fmt.Errorf("secure private sync span stage: %w", err)
	}
	stage.stats.ExternalArtifacts++
	pragmas := []string{
		"PRAGMA page_size=4096",
		"PRAGMA journal_mode=OFF",
		"PRAGMA synchronous=OFF",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA locking_mode=EXCLUSIVE",
		"PRAGMA mmap_size=0",
		fmt.Sprintf("PRAGMA cache_size=-%d", stage.options.SQLiteCacheKiB),
		fmt.Sprintf("PRAGMA max_page_count=%d", stage.maxSQLitePages),
	}
	for _, pragma := range pragmas {
		if _, err := conn.ExecContext(ctx, pragma); err != nil {
			return stage.handleSQLiteWriteError(ctx, fmt.Errorf("configure private sync span stage: %w", err))
		}
	}
	var journalMode string
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil || strings.ToLower(journalMode) != "off" {
		return errors.Join(&traceDBOutputInvariantError{Reason: "sync_span_stage_journal_not_disabled"}, err)
	}
	var mmapSize int64
	if err := conn.QueryRowContext(ctx, "PRAGMA mmap_size").Scan(&mmapSize); err != nil || mmapSize != 0 {
		return errors.Join(&traceDBOutputInvariantError{Reason: "sync_span_stage_mmap_not_disabled"}, err)
	}
	var pageSize int64
	if err := conn.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil || pageSize != traceDBSyncSpanSQLitePageBytes {
		return errors.Join(&traceDBOutputInvariantError{Reason: "sync_span_stage_page_size_not_applied"}, err)
	}
	var tempStore int64
	if err := conn.QueryRowContext(ctx, "PRAGMA temp_store").Scan(&tempStore); err != nil || tempStore != 2 {
		return errors.Join(&traceDBOutputInvariantError{Reason: "sync_span_stage_temp_store_not_memory"}, err)
	}
	var maxPages int64
	if err := conn.QueryRowContext(ctx, "PRAGMA max_page_count").Scan(&maxPages); err != nil || maxPages <= 0 || maxPages > stage.maxSQLitePages {
		return errors.Join(&traceDBOutputInvariantError{Reason: "sync_span_stage_page_cap_not_applied"}, err)
	}
	stage.maxSQLitePages = maxPages
	for _, ddl := range traceDBSyncSpanStageSchema {
		if _, err := conn.ExecContext(ctx, ddl); err != nil {
			return stage.handleSQLiteWriteError(ctx, fmt.Errorf("initialize private sync span stage: %w", err))
		}
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return stage.handleSQLiteWriteError(ctx, fmt.Errorf("begin private sync span stage: %w", err))
	}
	stage.tx = tx
	if stage.insertCandidate, err = tx.PrepareContext(ctx, traceDBSyncSpanInsertCandidateSQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if stage.insertIdentity, err = tx.PrepareContext(ctx, traceDBSyncSpanInsertIdentitySQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if stage.lookupIdentity, err = tx.PrepareContext(ctx, traceDBSyncSpanLookupIdentitySQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if stage.lookupSemantic, err = tx.PrepareContext(ctx, traceDBSyncSpanLookupSemanticSQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if stage.lookupInterval, err = tx.PrepareContext(ctx, traceDBSyncSpanLookupIntervalIdentitySQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if stage.lookupSemanticLocallyAdmitted, err = tx.PrepareContext(
		ctx, traceDBSyncSpanLookupSemanticLocallyAdmittedSQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if stage.lookupIntervalLocallyAdmitted, err = tx.PrepareContext(
		ctx, traceDBSyncSpanLookupIntervalLocallyAdmittedSQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if stage.censusIntervalLocallyAdmitted, err = tx.PrepareContext(
		ctx, traceDBSyncSpanCensusIntervalLocallyAdmittedSQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if stage.supersedeCPUUnavailable, err = tx.PrepareContext(
		ctx, traceDBSyncSpanSupersedeCPUUnavailableSQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if stage.selectCPUKnownCallstack, err = tx.PrepareContext(
		ctx, traceDBSyncSpanSelectCPUKnownCallstackSQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if stage.supersedeCPUKnownCallstack, err = tx.PrepareContext(
		ctx, traceDBSyncSpanSupersedeCPUKnownCallstackSQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if stage.upsertForced, err = tx.PrepareContext(ctx, traceDBSyncSpanUpsertForcedSQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if stage.insertFence, err = tx.PrepareContext(ctx, traceDBSyncSpanInsertFenceSQL); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	for _, staged := range stage.memoryCandidates {
		if err := stage.insertSQLiteCandidate(ctx, staged.Ordinal, staged.Candidate); err != nil {
			return err
		}
		if stage.budgetReason != "" {
			return nil
		}
	}
	for tid, reason := range stage.memoryForced {
		if err := stage.upsertSQLiteForced(ctx, tid, reason); err != nil {
			return err
		}
		if stage.budgetReason != "" {
			return nil
		}
	}
	for _, staged := range stage.memoryFences {
		if err := stage.insertSQLiteFence(ctx, staged.Ordinal, staged.Fence); err != nil {
			return err
		}
		if stage.budgetReason != "" {
			return nil
		}
	}
	stage.memoryCandidates = nil
	stage.memoryFences = nil
	stage.memoryIdentityFirst = nil
	stage.memoryForced = nil
	stage.residentBytes = 0
	stage.external = true
	stage.stats.Backend = "sqlite"
	return stage.sampleTempBytes()
}

var traceDBSyncSpanStageSchema = []string{
	`CREATE TABLE candidate (
		ordinal INTEGER PRIMARY KEY,
		superseded INTEGER NOT NULL CHECK(superseded IN (0,1)),
		lane_order_key BLOB NOT NULL CHECK(length(lane_order_key) = 47),
		zero_key INTEGER NOT NULL CHECK(zero_key IN (0,1)),
		producer INTEGER NOT NULL,
		stable_kind INTEGER NOT NULL,
		stable_id INTEGER NOT NULL,
		header_tid INTEGER NOT NULL,
		header_tgid INTEGER NOT NULL,
		marker_pid INTEGER NOT NULL,
		marker_known INTEGER NOT NULL CHECK(marker_known IN (0,1)),
		canonical_itid INTEGER NOT NULL,
		canonical_known INTEGER NOT NULL CHECK(canonical_known IN (0,1)),
		owner_ipid INTEGER NOT NULL,
		owner_known INTEGER NOT NULL CHECK(owner_known IN (0,1)),
		start_ns INTEGER NOT NULL,
		end_ns INTEGER NOT NULL,
		start_cpu INTEGER NOT NULL,
		end_cpu INTEGER NOT NULL,
		start_flags INTEGER NOT NULL,
		end_flags INTEGER NOT NULL,
		start_preempt_count INTEGER NOT NULL,
		end_preempt_count INTEGER NOT NULL,
		start_marker_body TEXT NOT NULL,
		end_marker_body TEXT NOT NULL,
		cpu_placement INTEGER NOT NULL,
		start_cpu_provenance INTEGER NOT NULL,
		end_cpu_provenance INTEGER NOT NULL,
		task TEXT NOT NULL,
		name TEXT NOT NULL,
		name_provenance INTEGER NOT NULL,
		depth INTEGER NOT NULL,
		depth_known INTEGER NOT NULL CHECK(depth_known IN (0,1)),
		depth_provenance INTEGER NOT NULL
	)`,
	`CREATE INDEX candidate_lane_idx ON candidate(header_tid, start_ns, zero_key, lane_order_key, ordinal)`,
	`CREATE TABLE identity_first (
		producer INTEGER NOT NULL,
		stable_kind INTEGER NOT NULL,
		stable_id INTEGER NOT NULL,
		first_header_tid INTEGER NOT NULL,
		PRIMARY KEY(producer, stable_kind, stable_id)
	) WITHOUT ROWID`,
	`CREATE TABLE forced_lane (
		header_tid INTEGER PRIMARY KEY,
		reason_mask INTEGER NOT NULL
	) WITHOUT ROWID`,
	`CREATE TABLE fence (
		ordinal INTEGER PRIMARY KEY,
		producer INTEGER NOT NULL,
		header_tid INTEGER NOT NULL,
		canonical_itid INTEGER NOT NULL,
		start_ns INTEGER NOT NULL,
		end_ns INTEGER NOT NULL,
		kind INTEGER NOT NULL,
		reason INTEGER NOT NULL
	)`,
	`CREATE INDEX fence_lane_idx ON fence(header_tid,start_ns,kind,end_ns,ordinal)`,
}

const traceDBSyncSpanInsertCandidateSQL = `INSERT INTO candidate(
	ordinal,superseded,lane_order_key,zero_key,producer,stable_kind,stable_id,header_tid,header_tgid,
	marker_pid,marker_known,canonical_itid,canonical_known,owner_ipid,owner_known,start_ns,end_ns,start_cpu,end_cpu,
	start_flags,end_flags,start_preempt_count,end_preempt_count,
	start_marker_body,end_marker_body,
	cpu_placement,start_cpu_provenance,end_cpu_provenance,task,name,name_provenance,depth,depth_known,depth_provenance
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
const traceDBSyncSpanInsertIdentitySQL = `INSERT OR IGNORE INTO identity_first VALUES(?,?,?,?)`
const traceDBSyncSpanLookupIdentitySQL = `SELECT first_header_tid FROM identity_first WHERE producer=? AND stable_kind=? AND stable_id=?`
const traceDBSyncSpanLookupSemanticSQL = `SELECT 1
	FROM candidate INDEXED BY candidate_lane_idx
	WHERE header_tid=? AND start_ns=? AND zero_key=1 AND end_ns=?
	  AND superseded=0
	  AND producer != ?
	  AND header_tgid=? AND CASE marker_known WHEN 1 THEN marker_pid ELSE header_tgid END=?
	  AND canonical_known=1 AND canonical_itid=?
	  AND owner_known=1 AND owner_ipid=?
	  AND name=?
	LIMIT 1`
const traceDBSyncSpanLookupIntervalIdentitySQL = `SELECT 1
	FROM candidate INDEXED BY candidate_lane_idx
	WHERE header_tid=? AND start_ns=? AND zero_key=1 AND end_ns=?
	  AND superseded=0
	  AND producer != ?
	  AND header_tgid=? AND CASE marker_known WHEN 1 THEN marker_pid ELSE header_tgid END=?
	  AND canonical_known=1 AND canonical_itid=?
	  AND owner_known=1 AND owner_ipid=?
	LIMIT 1`
const traceDBSyncSpanLookupSemanticLocallyAdmittedSQL = `SELECT 1
	FROM candidate AS c INDEXED BY candidate_lane_idx
	WHERE c.header_tid=? AND c.start_ns=? AND c.zero_key=1 AND c.end_ns=?
	  AND c.superseded=0
	  AND c.producer != ?
	  AND c.header_tgid=? AND CASE c.marker_known WHEN 1 THEN c.marker_pid ELSE c.header_tgid END=?
	  AND c.canonical_known=1 AND c.canonical_itid=?
	  AND c.owner_known=1 AND c.owner_ipid=?
	  AND c.name=?
	  AND NOT EXISTS (
		SELECT 1 FROM fence AS f
		WHERE f.header_tid=c.header_tid AND f.producer=c.producer
		  AND ((f.kind=? AND f.start_ns<c.end_ns AND f.end_ns>c.start_ns)
		    OR (f.kind=? AND c.end_ns>f.start_ns))
	  )
	  AND NOT (c.producer=? AND EXISTS (
		SELECT 1 FROM forced_lane AS p
		WHERE p.header_tid=c.header_tid AND (p.reason_mask & ?) != 0
	  ))
	LIMIT 1`
const traceDBSyncSpanLookupIntervalLocallyAdmittedSQL = `SELECT 1
	FROM candidate AS c INDEXED BY candidate_lane_idx
	WHERE c.header_tid=? AND c.start_ns=? AND c.zero_key=1 AND c.end_ns=?
	  AND c.superseded=0
	  AND c.producer != ?
	  AND c.header_tgid=? AND CASE c.marker_known WHEN 1 THEN c.marker_pid ELSE c.header_tgid END=?
	  AND c.canonical_known=1 AND c.canonical_itid=?
	  AND c.owner_known=1 AND c.owner_ipid=?
	  AND NOT EXISTS (
		SELECT 1 FROM fence AS f
		WHERE f.header_tid=c.header_tid AND f.producer=c.producer
		  AND ((f.kind=? AND f.start_ns<c.end_ns AND f.end_ns>c.start_ns)
		    OR (f.kind=? AND c.end_ns>f.start_ns))
	  )
	  AND NOT (c.producer=? AND EXISTS (
		SELECT 1 FROM forced_lane AS p
		WHERE p.header_tid=c.header_tid AND (p.reason_mask & ?) != 0
	  ))
	LIMIT 1`
const traceDBSyncSpanCensusIntervalLocallyAdmittedSQL = `SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN c.producer=? AND c.cpu_placement=? THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN c.producer=? AND c.cpu_placement!=? THEN 1 ELSE 0 END),0)
	FROM candidate AS c INDEXED BY candidate_lane_idx
	WHERE c.header_tid=? AND c.start_ns=? AND c.zero_key=1 AND c.end_ns=?
	  AND c.superseded=0
	  AND c.producer != ?
	  AND c.header_tgid=? AND CASE c.marker_known WHEN 1 THEN c.marker_pid ELSE c.header_tgid END=?
	  AND c.canonical_known=1 AND c.canonical_itid=?
	  AND c.owner_known=1 AND c.owner_ipid=?
	  AND NOT EXISTS (
		SELECT 1 FROM fence AS f
		WHERE f.header_tid=c.header_tid AND f.producer=c.producer
		  AND ((f.kind=? AND f.start_ns<c.end_ns AND f.end_ns>c.start_ns)
		    OR (f.kind=? AND c.end_ns>f.start_ns))
	  )
	  AND NOT (c.producer=? AND EXISTS (
		SELECT 1 FROM forced_lane AS p
		WHERE p.header_tid=c.header_tid AND (p.reason_mask & ?) != 0
	  ))`
const traceDBSyncSpanSupersedeCPUUnavailableSQL = `UPDATE candidate AS c
	SET superseded=1
	WHERE c.header_tid=? AND c.start_ns=? AND c.zero_key=1 AND c.end_ns=?
	  AND c.superseded=0
	  AND c.producer=? AND c.cpu_placement!=?
	  AND c.header_tgid=? AND CASE c.marker_known WHEN 1 THEN c.marker_pid ELSE c.header_tgid END=?
	  AND c.canonical_known=1 AND c.canonical_itid=?
	  AND c.owner_known=1 AND c.owner_ipid=?
	  AND NOT EXISTS (
		SELECT 1 FROM fence AS f
		WHERE f.header_tid=c.header_tid AND f.producer=c.producer
		  AND ((f.kind=? AND f.start_ns<c.end_ns AND f.end_ns>c.start_ns)
		    OR (f.kind=? AND c.end_ns>f.start_ns))
	  )
	  AND NOT EXISTS (
		SELECT 1 FROM forced_lane AS p
		WHERE p.header_tid=c.header_tid AND (p.reason_mask & ?) != 0
	  )`
const traceDBSyncSpanSelectCPUKnownCallstackSQL = `SELECT
		c.name, CASE c.marker_known WHEN 1 THEN c.marker_pid ELSE c.header_tgid END
	FROM candidate AS c INDEXED BY candidate_lane_idx
	WHERE c.header_tid=? AND c.start_ns=? AND c.zero_key=1 AND c.end_ns=?
	  AND c.superseded=0
	  AND c.producer=? AND c.cpu_placement=?
	  AND c.header_tgid=? AND CASE c.marker_known WHEN 1 THEN c.marker_pid ELSE c.header_tgid END=?
	  AND c.canonical_known=1 AND c.canonical_itid=?
	  AND c.owner_known=1 AND c.owner_ipid=?
	  AND NOT EXISTS (
		SELECT 1 FROM fence AS f
		WHERE f.header_tid=c.header_tid AND f.producer=c.producer
		  AND ((f.kind=? AND f.start_ns<c.end_ns AND f.end_ns>c.start_ns)
		    OR (f.kind=? AND c.end_ns>f.start_ns))
	  )
	  AND NOT EXISTS (
		SELECT 1 FROM forced_lane AS p
		WHERE p.header_tid=c.header_tid AND (p.reason_mask & ?) != 0
	  )`
const traceDBSyncSpanSupersedeCPUKnownCallstackSQL = `UPDATE candidate AS c
	SET superseded=1
	WHERE c.header_tid=? AND c.start_ns=? AND c.zero_key=1 AND c.end_ns=?
	  AND c.superseded=0
	  AND c.producer=? AND c.cpu_placement=?
	  AND c.header_tgid=? AND CASE c.marker_known WHEN 1 THEN c.marker_pid ELSE c.header_tgid END=?
	  AND c.canonical_known=1 AND c.canonical_itid=?
	  AND c.owner_known=1 AND c.owner_ipid=?
	  AND NOT EXISTS (
		SELECT 1 FROM fence AS f
		WHERE f.header_tid=c.header_tid AND f.producer=c.producer
		  AND ((f.kind=? AND f.start_ns<c.end_ns AND f.end_ns>c.start_ns)
		    OR (f.kind=? AND c.end_ns>f.start_ns))
	  )
	  AND NOT EXISTS (
		SELECT 1 FROM forced_lane AS p
		WHERE p.header_tid=c.header_tid AND (p.reason_mask & ?) != 0
	  )`
const traceDBSyncSpanUpsertForcedSQL = `INSERT INTO forced_lane(header_tid,reason_mask) VALUES(?,?)
	ON CONFLICT(header_tid) DO UPDATE SET reason_mask = reason_mask | excluded.reason_mask`
const traceDBSyncSpanInsertFenceSQL = `INSERT INTO fence(
	ordinal,producer,header_tid,canonical_itid,start_ns,end_ns,kind,reason
) VALUES(?,?,?,?,?,?,?,?)`

func (stage *traceDBSyncSpanStage) insertSQLiteCandidate(ctx context.Context, ordinal int64, candidate traceDBSyncSpanCandidate) error {
	if stage.budgetReason != "" {
		return nil
	}
	payloadBytes := int64(len(candidate.Task)) + int64(len(candidate.Name)) +
		int64(len(candidate.StartMarkerBody)) + int64(len(candidate.EndMarkerBody))
	if payloadBytes > (math.MaxInt64-1024)/2 {
		return stage.failBudget(traceDBSyncSpanStageBudgetSQLitePageCap)
	}
	estimatedBytes := (payloadBytes + 1024) * 2
	requiredPages := (estimatedBytes+traceDBSyncSpanSQLitePageBytes-1)/traceDBSyncSpanSQLitePageBytes +
		traceDBSyncSpanSQLiteWriteReservePages
	if err := stage.ensureSQLitePageHeadroom(ctx, requiredPages); err != nil {
		return err
	}
	key := traceDBSyncSpanLaneOrderKey(candidate)
	zeroFirstKey := boolToSQLiteInt(candidate.Start != candidate.End)
	result, err := stage.insertCandidate.ExecContext(ctx,
		ordinal, 0, key, zeroFirstKey,
		candidate.Producer, candidate.StableKind, candidate.StableID,
		candidate.HeaderTID, candidate.HeaderTGID,
		candidate.MarkerPID, boolToSQLiteInt(candidate.MarkerPIDKnown),
		candidate.CanonicalITID, boolToSQLiteInt(candidate.CanonicalITIDKnown),
		candidate.OwnerIPID, boolToSQLiteInt(candidate.OwnerIPIDKnown),
		candidate.Start, candidate.End, candidate.StartCPU, candidate.EndCPU,
		candidate.StartFlags, candidate.EndFlags,
		candidate.StartPreemptCount, candidate.EndPreemptCount,
		candidate.StartMarkerBody, candidate.EndMarkerBody,
		candidate.CPUPlacement,
		candidate.StartCPUProvenance, candidate.EndCPUProvenance,
		candidate.Task, candidate.Name, candidate.NameProvenance,
		candidate.Depth, boolToSQLiteInt(candidate.DepthKnown), candidate.DepthProvenance,
	)
	if err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return errors.Join(&traceDBOutputInvariantError{Reason: "sync_span_stage_candidate_insert_count"}, rowsErr)
	}
	identityResult, err := stage.insertIdentity.ExecContext(ctx,
		candidate.Producer, candidate.StableKind, candidate.StableID, candidate.HeaderTID)
	if err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	rows, err := identityResult.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 1 {
		return nil
	}
	if rows != 0 {
		return &traceDBOutputInvariantError{Reason: "sync_span_stage_identity_insert_count"}
	}
	var firstTID int64
	if err := stage.lookupIdentity.QueryRowContext(ctx,
		candidate.Producer, candidate.StableKind, candidate.StableID).Scan(&firstTID); err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if err := stage.upsertSQLiteForced(ctx, firstTID, traceDBSyncSpanForcedDuplicate); err != nil {
		return err
	}
	return stage.upsertSQLiteForced(ctx, candidate.HeaderTID, traceDBSyncSpanForcedDuplicate)
}

func (stage *traceDBSyncSpanStage) hasSemanticCandidate(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (bool, bool, error) {
	if err := stage.requireOpen(ctx); err != nil {
		return false, false, err
	}
	key, ok := traceDBSyncSpanCandidateSemanticKey(candidate)
	if !ok {
		return false, true, nil
	}
	if stage.budgetReason != "" {
		return false, false, nil
	}
	if !stage.external {
		if err := stage.promoteToSQLite(ctx); err != nil {
			if _, pure := traceDBSyncSpanPureBudgetReason(err); pure {
				return false, false, nil
			}
			return false, false, err
		}
	}
	if stage.budgetReason != "" || stage.lookupSemantic == nil {
		return false, false, nil
	}
	var one int
	err := stage.lookupSemantic.QueryRowContext(
		ctx, key.HeaderTID, key.Start, key.End,
		traceDBSyncSpanProducerSourceRawMarker,
		key.HeaderTGID, key.MarkerPID, key.CanonicalITID, key.OwnerIPID, key.Name,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, true, nil
	}
	if err != nil {
		return false, false, stage.handleSQLiteWriteError(ctx,
			fmt.Errorf("lookup exact sync span semantic candidate: %w", err))
	}
	return one == 1, true, nil
}

func (stage *traceDBSyncSpanStage) hasIntervalIdentityCandidate(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (bool, bool, error) {
	if err := stage.requireOpen(ctx); err != nil {
		return false, false, err
	}
	key, ok := traceDBSyncSpanCandidateSemanticKey(candidate)
	if !ok {
		return false, true, nil
	}
	if stage.budgetReason != "" {
		return false, false, nil
	}
	if !stage.external {
		if err := stage.promoteToSQLite(ctx); err != nil {
			if _, pure := traceDBSyncSpanPureBudgetReason(err); pure {
				return false, false, nil
			}
			return false, false, err
		}
	}
	if stage.budgetReason != "" || stage.lookupInterval == nil {
		return false, false, nil
	}
	var one int
	err := stage.lookupInterval.QueryRowContext(
		ctx, key.HeaderTID, key.Start, key.End,
		traceDBSyncSpanProducerSourceRawMarker,
		key.HeaderTGID, key.MarkerPID, key.CanonicalITID, key.OwnerIPID,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, true, nil
	}
	if err != nil {
		return false, false, stage.handleSQLiteWriteError(ctx,
			fmt.Errorf("lookup exact sync span interval identity candidate: %w", err))
	}
	return one == 1, true, nil
}

func (stage *traceDBSyncSpanStage) hasLocallyAdmittedSemanticCandidate(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (bool, bool, error) {
	if err := stage.requireOpen(ctx); err != nil {
		return false, false, err
	}
	key, ok := traceDBSyncSpanCandidateSemanticKey(candidate)
	if !ok {
		return false, true, nil
	}
	if stage.budgetReason != "" {
		return false, false, nil
	}
	if !stage.external {
		if err := stage.promoteToSQLite(ctx); err != nil {
			if _, pure := traceDBSyncSpanPureBudgetReason(err); pure {
				return false, false, nil
			}
			return false, false, err
		}
	}
	if stage.budgetReason != "" || stage.lookupSemanticLocallyAdmitted == nil {
		return false, false, nil
	}
	var one int
	err := stage.lookupSemanticLocallyAdmitted.QueryRowContext(
		ctx, key.HeaderTID, key.Start, key.End,
		traceDBSyncSpanProducerSourceRawMarker,
		key.HeaderTGID, key.MarkerPID, key.CanonicalITID, key.OwnerIPID, key.Name,
		traceDBSyncSpanFenceInterval, traceDBSyncSpanFenceSuffix,
		traceDBSyncSpanProducerCallstack, traceDBSyncSpanForcedCallstackPoison,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, true, nil
	}
	if err != nil {
		return false, false, stage.handleSQLiteWriteError(ctx,
			fmt.Errorf("lookup locally admitted exact sync span semantic candidate: %w", err))
	}
	return one == 1, true, nil
}

func (stage *traceDBSyncSpanStage) hasLocallyAdmittedIntervalIdentityCandidate(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (bool, bool, error) {
	if err := stage.requireOpen(ctx); err != nil {
		return false, false, err
	}
	key, ok := traceDBSyncSpanCandidateSemanticKey(candidate)
	if !ok {
		return false, true, nil
	}
	if stage.budgetReason != "" {
		return false, false, nil
	}
	if !stage.external {
		if err := stage.promoteToSQLite(ctx); err != nil {
			if _, pure := traceDBSyncSpanPureBudgetReason(err); pure {
				return false, false, nil
			}
			return false, false, err
		}
	}
	if stage.budgetReason != "" || stage.lookupIntervalLocallyAdmitted == nil {
		return false, false, nil
	}
	var one int
	err := stage.lookupIntervalLocallyAdmitted.QueryRowContext(
		ctx, key.HeaderTID, key.Start, key.End,
		traceDBSyncSpanProducerSourceRawMarker,
		key.HeaderTGID, key.MarkerPID, key.CanonicalITID, key.OwnerIPID,
		traceDBSyncSpanFenceInterval, traceDBSyncSpanFenceSuffix,
		traceDBSyncSpanProducerCallstack, traceDBSyncSpanForcedCallstackPoison,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, true, nil
	}
	if err != nil {
		return false, false, stage.handleSQLiteWriteError(ctx,
			fmt.Errorf("lookup locally admitted exact sync span interval identity candidate: %w", err))
	}
	return one == 1, true, nil
}

func (stage *traceDBSyncSpanStage) censusLocallyAdmittedIntervalIdentityCandidates(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (traceDBSyncSpanIntervalCollisionCensus, bool, error) {
	if err := stage.requireOpen(ctx); err != nil {
		return traceDBSyncSpanIntervalCollisionCensus{}, false, err
	}
	key, ok := traceDBSyncSpanCandidateSemanticKey(candidate)
	if !ok {
		return traceDBSyncSpanIntervalCollisionCensus{}, true, nil
	}
	if stage.budgetReason != "" {
		return traceDBSyncSpanIntervalCollisionCensus{}, false, nil
	}
	if !stage.external {
		if err := stage.promoteToSQLite(ctx); err != nil {
			if _, pure := traceDBSyncSpanPureBudgetReason(err); pure {
				return traceDBSyncSpanIntervalCollisionCensus{}, false, nil
			}
			return traceDBSyncSpanIntervalCollisionCensus{}, false, err
		}
	}
	if stage.budgetReason != "" || stage.censusIntervalLocallyAdmitted == nil {
		return traceDBSyncSpanIntervalCollisionCensus{}, false, nil
	}
	var census traceDBSyncSpanIntervalCollisionCensus
	err := stage.censusIntervalLocallyAdmitted.QueryRowContext(
		ctx,
		traceDBSyncSpanProducerCallstack, traceDBSyncSpanCPUPlacementKnown,
		traceDBSyncSpanProducerCallstack, traceDBSyncSpanCPUPlacementKnown,
		key.HeaderTID, key.Start, key.End,
		traceDBSyncSpanProducerSourceRawMarker,
		key.HeaderTGID, key.MarkerPID, key.CanonicalITID, key.OwnerIPID,
		traceDBSyncSpanFenceInterval, traceDBSyncSpanFenceSuffix,
		traceDBSyncSpanProducerCallstack, traceDBSyncSpanForcedCallstackPoison,
	).Scan(&census.Total, &census.CallstackCPUKnown, &census.CallstackCPUUnavailable)
	if err != nil {
		return traceDBSyncSpanIntervalCollisionCensus{}, false,
			stage.handleSQLiteWriteError(ctx,
				fmt.Errorf("census locally admitted exact sync span interval candidates: %w", err))
	}
	if census.Total < 0 || census.CallstackCPUKnown < 0 ||
		census.CallstackCPUUnavailable < 0 ||
		census.CallstackCPUKnown > census.Total ||
		census.CallstackCPUUnavailable > census.Total-census.CallstackCPUKnown {
		return traceDBSyncSpanIntervalCollisionCensus{}, false,
			&traceDBOutputInvariantError{Reason: "invalid_sync_span_interval_collision_census"}
	}
	return census, true, nil
}

func (stage *traceDBSyncSpanStage) supersedeUniqueLocallyAdmittedCPUUnavailableCallstackCandidate(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (bool, bool, error) {
	if err := stage.requireOpen(ctx); err != nil {
		return false, false, err
	}
	key, ok := traceDBSyncSpanCandidateSemanticKey(candidate)
	if !ok {
		return false, true, nil
	}
	if stage.budgetReason != "" {
		return false, false, nil
	}
	if !stage.external {
		if err := stage.promoteToSQLite(ctx); err != nil {
			if _, pure := traceDBSyncSpanPureBudgetReason(err); pure {
				return false, false, nil
			}
			return false, false, err
		}
	}
	if stage.budgetReason != "" || stage.supersedeCPUUnavailable == nil {
		return false, false, nil
	}
	if err := stage.ensureSQLitePageHeadroom(ctx,
		traceDBSyncSpanSQLiteWriteReservePages); err != nil {
		if _, pure := traceDBSyncSpanPureBudgetReason(err); pure {
			return false, false, nil
		}
		return false, false, err
	}
	result, err := stage.supersedeCPUUnavailable.ExecContext(
		ctx,
		key.HeaderTID, key.Start, key.End,
		traceDBSyncSpanProducerCallstack, traceDBSyncSpanCPUPlacementKnown,
		key.HeaderTGID, key.MarkerPID, key.CanonicalITID, key.OwnerIPID,
		traceDBSyncSpanFenceInterval, traceDBSyncSpanFenceSuffix,
		traceDBSyncSpanForcedCallstackPoison,
	)
	if err != nil {
		return false, false, stage.handleSQLiteWriteError(ctx,
			fmt.Errorf("supersede unique CPU-unavailable callstack candidate: %w", err))
	}
	rows, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return false, false, rowsErr
	}
	if rows == 0 {
		return false, true, nil
	}
	if rows != 1 {
		return false, false,
			&traceDBOutputInvariantError{Reason: "sync_span_cpu_unavailable_supersede_count"}
	}
	return true, true, nil
}

func (stage *traceDBSyncSpanStage) supersedeUniqueLocallyAdmittedNameUnrepresentableCallstackCandidate(
	ctx context.Context,
	candidate traceDBSyncSpanCandidate,
) (bool, bool, error) {
	if err := stage.requireOpen(ctx); err != nil {
		return false, false, err
	}
	key, ok := traceDBSyncSpanCandidateSemanticKey(candidate)
	if !ok {
		return false, true, nil
	}
	if stage.budgetReason != "" {
		return false, false, nil
	}
	if !stage.external {
		if err := stage.promoteToSQLite(ctx); err != nil {
			if _, pure := traceDBSyncSpanPureBudgetReason(err); pure {
				return false, false, nil
			}
			return false, false, err
		}
	}
	if stage.budgetReason != "" || stage.selectCPUKnownCallstack == nil ||
		stage.supersedeCPUKnownCallstack == nil {
		return false, false, nil
	}
	args := []any{
		key.HeaderTID, key.Start, key.End,
		traceDBSyncSpanProducerCallstack, traceDBSyncSpanCPUPlacementKnown,
		key.HeaderTGID, key.MarkerPID, key.CanonicalITID, key.OwnerIPID,
		traceDBSyncSpanFenceInterval, traceDBSyncSpanFenceSuffix,
		traceDBSyncSpanForcedCallstackPoison,
	}
	rows, err := stage.selectCPUKnownCallstack.QueryContext(ctx, args...)
	if err != nil {
		return false, false, stage.handleSQLiteWriteError(ctx,
			fmt.Errorf("select unique CPU-known callstack candidate: %w", err))
	}
	var name string
	var markerPID int64
	count := 0
	for rows.Next() {
		count++
		if count > 1 {
			_ = rows.Close()
			return false, false, &traceDBOutputInvariantError{
				Reason: "sync_span_cpu_known_candidate_not_unique",
			}
		}
		if err := rows.Scan(&name, &markerPID); err != nil {
			_ = rows.Close()
			return false, false, err
		}
	}
	if err := rows.Close(); err != nil {
		return false, false, err
	}
	if count == 0 {
		return false, true, nil
	}
	if markerPID <= 0 || markerPID > math.MaxInt32 {
		return false, false, &traceDBOutputInvariantError{
			Reason: "sync_span_cpu_known_candidate_invalid_marker_pid",
		}
	}
	if tracequery.StandardSyncTraceMarkNameRepresentable(int(markerPID), name) {
		return false, true, nil
	}
	if err := stage.ensureSQLitePageHeadroom(ctx,
		traceDBSyncSpanSQLiteWriteReservePages); err != nil {
		if _, pure := traceDBSyncSpanPureBudgetReason(err); pure {
			return false, false, nil
		}
		return false, false, err
	}
	result, err := stage.supersedeCPUKnownCallstack.ExecContext(ctx, args...)
	if err != nil {
		return false, false, stage.handleSQLiteWriteError(ctx,
			fmt.Errorf("supersede unique name-unrepresentable callstack candidate: %w", err))
	}
	affected, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return false, false, rowsErr
	}
	if affected == 0 {
		return false, true, nil
	}
	if affected != 1 {
		return false, false, &traceDBOutputInvariantError{
			Reason: "sync_span_name_unrepresentable_supersede_count",
		}
	}
	return true, true, nil
}

func (stage *traceDBSyncSpanStage) upsertSQLiteForced(ctx context.Context, tid int64, reason traceDBSyncSpanForcedReason) error {
	if stage.budgetReason != "" {
		return nil
	}
	if err := stage.ensureSQLitePageHeadroom(ctx, traceDBSyncSpanSQLiteForcedReservePages); err != nil {
		return err
	}
	result, err := stage.upsertForced.ExecContext(ctx, tid, reason)
	if err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return errors.Join(&traceDBOutputInvariantError{Reason: "sync_span_stage_forced_insert_count"}, rowsErr)
	}
	return nil
}

func (stage *traceDBSyncSpanStage) insertSQLiteFence(ctx context.Context, ordinal int64, fence traceDBSyncSpanLaneFence) error {
	if stage.budgetReason != "" {
		return nil
	}
	if err := stage.ensureSQLitePageHeadroom(ctx, traceDBSyncSpanSQLiteForcedReservePages); err != nil {
		return err
	}
	result, err := stage.insertFence.ExecContext(ctx,
		ordinal, fence.Producer, fence.HeaderTID, fence.CanonicalITID,
		fence.Start, fence.End, fence.Kind, fence.Reason)
	if err != nil {
		return stage.handleSQLiteWriteError(ctx, err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return errors.Join(&traceDBOutputInvariantError{Reason: "sync_span_stage_fence_insert_count"}, rowsErr)
	}
	return nil
}

func (stage *traceDBSyncSpanStage) ensureSQLitePageHeadroom(ctx context.Context, requiredPages int64) error {
	if requiredPages <= 0 || stage.maxSQLitePages <= 0 {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_page_reservation"}
	}
	var pages int64
	var err error
	if stage.tx != nil {
		err = stage.tx.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages)
	} else if stage.conn != nil {
		err = stage.conn.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages)
	} else {
		return &traceDBOutputInvariantError{Reason: "missing_sync_span_stage_page_reservation_connection"}
	}
	if err != nil {
		return fmt.Errorf("read SQLite page count for configured headroom: %w", err)
	}
	pageBytes := pages * traceDBSyncSpanSQLitePageBytes
	if pageBytes > stage.tempAccountedBytes {
		stage.tempAccountedBytes = pageBytes
	}
	if pageBytes > stage.stats.PeakTempBytes {
		stage.stats.PeakTempBytes = pageBytes
	}
	if requiredPages > stage.maxSQLitePages || pages > stage.maxSQLitePages-requiredPages {
		return stage.failBudget(traceDBSyncSpanStageBudgetSQLitePageCap)
	}
	return nil
}

func boolToSQLiteInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func (stage *traceDBSyncSpanStage) handleSQLiteWriteError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return errors.Join(ctxErr, err)
	}
	if traceDBSQLitePrimaryErrorCode(err) == 13 {
		atCap, proofErr := stage.sqliteAtConfiguredPageCap(ctx)
		if proofErr != nil {
			return errors.Join(fmt.Errorf("private sync span stage write: %w", err), proofErr)
		}
		if atCap {
			return stage.failBudget(traceDBSyncSpanStageBudgetSQLitePageCap)
		}
	}
	return fmt.Errorf("private sync span stage write: %w", err)
}

func traceDBSQLitePrimaryErrorCode(err error) int {
	type errorCoder interface{ Code() int }
	var coded errorCoder
	if !errors.As(err, &coded) {
		return 0
	}
	return coded.Code() & 0xff
}

func (stage *traceDBSyncSpanStage) sqliteAtConfiguredPageCap(ctx context.Context) (bool, error) {
	if stage.maxSQLitePages <= 0 {
		return false, nil
	}
	var pages int64
	if stage.tx != nil {
		if err := stage.tx.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err == nil {
			return pages >= stage.maxSQLitePages, nil
		}
		statementErr := stage.closeSQLiteWriteStatements()
		tx := stage.tx
		stage.tx = nil
		rollbackErr := tx.Rollback()
		if errors.Is(rollbackErr, sql.ErrTxDone) || traceDBSQLiteNoActiveTransaction(rollbackErr) {
			rollbackErr = nil
		}
		if statementErr != nil || rollbackErr != nil {
			if statementErr != nil {
				statementErr = fmt.Errorf("close SQLite write statements after FULL: %w", statementErr)
			}
			return false, errors.Join(statementErr, rollbackErr)
		}
	}
	if stage.conn != nil {
		if err := stage.conn.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err == nil {
			return pages >= stage.maxSQLitePages, nil
		} else {
			return false, fmt.Errorf("read SQLite page count after FULL: %w", err)
		}
	}
	return false, nil
}

func traceDBSQLiteNoActiveTransaction(err error) bool {
	return err != nil && traceDBSQLitePrimaryErrorCode(err) == 1 &&
		strings.Contains(strings.ToLower(err.Error()), "no transaction is active")
}

func (stage *traceDBSyncSpanStage) failBudget(reason string) error {
	if stage.budgetReason == "" {
		stage.budgetReason = reason
	}
	return &traceDBSyncSpanStageBudgetError{Reason: stage.budgetReason}
}

func (stage *traceDBSyncSpanStage) discardBackend() error {
	var errs []error
	for _, statement := range []*sql.Stmt{
		stage.insertCandidate, stage.insertIdentity, stage.lookupIdentity,
		stage.lookupSemantic, stage.lookupInterval,
		stage.lookupSemanticLocallyAdmitted, stage.lookupIntervalLocallyAdmitted,
		stage.censusIntervalLocallyAdmitted, stage.supersedeCPUUnavailable,
		stage.selectCPUKnownCallstack, stage.supersedeCPUKnownCallstack,
		stage.upsertForced, stage.insertFence,
	} {
		if statement != nil {
			errs = append(errs, statement.Close())
		}
	}
	stage.insertCandidate, stage.insertIdentity, stage.lookupIdentity = nil, nil, nil
	stage.lookupSemantic, stage.lookupInterval = nil, nil
	stage.lookupSemanticLocallyAdmitted, stage.lookupIntervalLocallyAdmitted = nil, nil
	stage.censusIntervalLocallyAdmitted = nil
	stage.supersedeCPUUnavailable = nil
	stage.selectCPUKnownCallstack, stage.supersedeCPUKnownCallstack = nil, nil
	stage.upsertForced, stage.insertFence = nil, nil
	if stage.tx != nil {
		err := stage.tx.Rollback()
		if !errors.Is(err, sql.ErrTxDone) && !traceDBSQLiteNoActiveTransaction(err) {
			errs = append(errs, err)
		}
		stage.tx = nil
	}
	if stage.conn != nil {
		errs = append(errs, stage.conn.Close())
		stage.conn = nil
	}
	if stage.db != nil {
		errs = append(errs, stage.db.Close())
		stage.db = nil
	}
	if stage.dbPath != "" {
		for _, path := range []string{stage.dbPath, stage.dbPath + "-journal", stage.dbPath + "-wal", stage.dbPath + "-shm"} {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	stage.external = false
	stage.dbPath = ""
	return errors.Join(errs...)
}

func (stage *traceDBSyncSpanStage) sampleTempBytes() error {
	entries, err := os.ReadDir(stage.workspace)
	if err != nil {
		return err
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > math.MaxInt64-total {
			return stage.failBudget(traceDBSyncSpanStageBudgetTempByteCap)
		}
		total += info.Size()
	}
	if total > stage.stats.PeakTempBytes {
		stage.stats.PeakTempBytes = total
	}
	if total > stage.tempAccountedBytes {
		stage.tempAccountedBytes = total
	}
	if total > stage.options.MaxTempBytes {
		return stage.failBudget(traceDBSyncSpanStageBudgetTempByteCap)
	}
	return nil
}

func (stage *traceDBSyncSpanStage) reserveTempBytes(delta int64) error {
	if delta < 0 || stage.tempAccountedBytes > stage.options.MaxTempBytes-delta {
		return stage.failBudget(traceDBSyncSpanStageBudgetTempByteCap)
	}
	stage.tempAccountedBytes += delta
	if stage.tempAccountedBytes > stage.stats.PeakTempBytes {
		stage.stats.PeakTempBytes = stage.tempAccountedBytes
	}
	return nil
}

func traceDBSyncSpanLaneCandidateLess(left, right traceDBSyncSpanCandidate) bool {
	if left.HeaderTID != right.HeaderTID {
		return left.HeaderTID < right.HeaderTID
	}
	if left.Start != right.Start {
		return left.Start < right.Start
	}
	leftZero, rightZero := left.Start == left.End, right.Start == right.End
	if leftZero != rightZero {
		return leftZero
	}
	if leftZero {
		return traceDBSyncSpanStableLess(left, right)
	}
	if left.End != right.End {
		return left.End > right.End
	}
	return traceDBSyncSpanCandidateTypedTieLess(left, right)
}

func traceDBSyncSpanFenceLess(left, right traceDBSyncSpanLaneFence) bool {
	if left.HeaderTID != right.HeaderTID {
		return left.HeaderTID < right.HeaderTID
	}
	if left.Start != right.Start {
		return left.Start < right.Start
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.End != right.End {
		return left.End < right.End
	}
	if left.Producer != right.Producer {
		return left.Producer < right.Producer
	}
	return left.CanonicalITID < right.CanonicalITID
}

func traceDBSyncSpanCandidateTypedTieLess(left, right traceDBSyncSpanCandidate) bool {
	if left.Producer != right.Producer {
		return left.Producer < right.Producer
	}
	if left.StableKind != right.StableKind {
		return left.StableKind < right.StableKind
	}
	if left.CanonicalITIDKnown != right.CanonicalITIDKnown {
		return left.CanonicalITIDKnown
	}
	if left.CanonicalITID != right.CanonicalITID {
		return left.CanonicalITID < right.CanonicalITID
	}
	if left.OwnerIPIDKnown != right.OwnerIPIDKnown {
		return left.OwnerIPIDKnown
	}
	if left.OwnerIPID != right.OwnerIPID {
		return left.OwnerIPID < right.OwnerIPID
	}
	if left.DepthKnown != right.DepthKnown {
		return left.DepthKnown
	}
	if left.DepthProvenance != right.DepthProvenance {
		return left.DepthProvenance < right.DepthProvenance
	}
	if left.Depth != right.Depth {
		return left.Depth < right.Depth
	}
	return traceDBSyncSpanStableLess(left, right)
}

func traceDBSyncSpanLaneOrderKey(candidate traceDBSyncSpanCandidate) []byte {
	key := make([]byte, 0, traceDBSyncSpanLaneOrderKeyBytes)
	key = append(key, 1)
	if candidate.Start == candidate.End {
		key = append(key, byte(candidate.Producer), byte(candidate.StableKind))
		key = appendTraceDBOrderInt64(key, candidate.StableID, false)
		for len(key) < traceDBSyncSpanLaneOrderKeyBytes {
			key = append(key, 0)
		}
		return key
	}
	key = appendTraceDBOrderInt64(key, candidate.End, true)
	key = append(key, byte(candidate.Producer), byte(candidate.StableKind))
	key = append(key, traceDBKnownFirstKey(candidate.CanonicalITIDKnown))
	key = appendTraceDBOrderInt64(key, candidate.CanonicalITID, false)
	key = append(key, traceDBKnownFirstKey(candidate.OwnerIPIDKnown))
	key = appendTraceDBOrderInt64(key, candidate.OwnerIPID, false)
	key = append(key, traceDBKnownFirstKey(candidate.DepthKnown), byte(candidate.DepthProvenance))
	key = appendTraceDBOrderInt64(key, candidate.Depth, false)
	key = appendTraceDBOrderInt64(key, candidate.StableID, false)
	if len(key) != traceDBSyncSpanLaneOrderKeyBytes {
		panic("sync span lane order key width drift")
	}
	return key
}

func traceDBKnownFirstKey(known bool) byte {
	if known {
		return 0
	}
	return 1
}

func appendTraceDBOrderInt64(dst []byte, value int64, descending bool) []byte {
	encoded := uint64(value) ^ (uint64(1) << 63)
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], encoded)
	if descending {
		for i := range raw {
			raw[i] = ^raw[i]
		}
	}
	return append(dst, raw[:]...)
}

const traceDBSyncSpanSelectCandidatesSQL = `SELECT
	ordinal,lane_order_key,zero_key,producer,stable_kind,stable_id,header_tid,header_tgid,
	marker_pid,marker_known,canonical_itid,canonical_known,owner_ipid,owner_known,start_ns,end_ns,start_cpu,end_cpu,
	start_flags,end_flags,start_preempt_count,end_preempt_count,
	start_marker_body,end_marker_body,
	cpu_placement,start_cpu_provenance,end_cpu_provenance,task,name,name_provenance,depth,depth_known,depth_provenance
FROM candidate INDEXED BY candidate_lane_idx
WHERE superseded=0
ORDER BY header_tid,start_ns,zero_key,lane_order_key,ordinal`

const traceDBSyncSpanSelectForcedSQL = `SELECT header_tid,reason_mask FROM forced_lane ORDER BY header_tid`
const traceDBSyncSpanSelectFencesSQL = `SELECT
	ordinal,producer,header_tid,canonical_itid,start_ns,end_ns,kind,reason
FROM fence INDEXED BY fence_lane_idx
ORDER BY header_tid,start_ns,kind,end_ns,ordinal`

func (stage *traceDBSyncSpanStage) seal(ctx context.Context) error {
	if err := stage.requireOpen(ctx); err != nil {
		return err
	}
	if stage.budgetReason != "" {
		return &traceDBSyncSpanStageBudgetError{Reason: stage.budgetReason}
	}
	// Sorting forced TIDs for the memory iterator needs one int64 per lane. If
	// that transient index would cross the resident payload cap, promote first.
	if !stage.external && int64(len(stage.memoryForced))*8 > stage.options.ResidentBytes-stage.residentBytes {
		if err := stage.promoteToSQLite(ctx); err != nil {
			return err
		}
	}
	if stage.budgetReason != "" {
		return &traceDBSyncSpanStageBudgetError{Reason: stage.budgetReason}
	}
	if stage.external {
		if err := stage.closeSQLiteWriteStatements(); err != nil {
			return err
		}
		tx := stage.tx
		stage.tx = nil
		if tx == nil {
			return &traceDBOutputInvariantError{Reason: "sync_span_stage_missing_write_transaction"}
		}
		if err := tx.Commit(); err != nil {
			return stage.handleSQLiteWriteError(ctx, fmt.Errorf("commit private sync span stage: %w", err))
		}
		if err := stage.sampleTempBytes(); err != nil {
			return err
		}
		if err := stage.verifySQLiteLanePlan(ctx); err != nil {
			return err
		}
	} else {
		sort.SliceStable(stage.memoryCandidates, func(i, j int) bool {
			left, right := stage.memoryCandidates[i], stage.memoryCandidates[j]
			if traceDBSyncSpanLaneCandidateLess(left.Candidate, right.Candidate) {
				return true
			}
			if traceDBSyncSpanLaneCandidateLess(right.Candidate, left.Candidate) {
				return false
			}
			return left.Ordinal < right.Ordinal
		})
		sort.SliceStable(stage.memoryFences, func(i, j int) bool {
			left, right := stage.memoryFences[i], stage.memoryFences[j]
			if traceDBSyncSpanFenceLess(left.Fence, right.Fence) {
				return true
			}
			if traceDBSyncSpanFenceLess(right.Fence, left.Fence) {
				return false
			}
			return left.Ordinal < right.Ordinal
		})
	}
	stage.sealed = true
	return nil
}

func (stage *traceDBSyncSpanStage) closeSQLiteWriteStatements() error {
	var errs []error
	for _, statement := range []*sql.Stmt{
		stage.insertCandidate, stage.insertIdentity, stage.lookupIdentity,
		stage.lookupSemantic, stage.lookupInterval,
		stage.lookupSemanticLocallyAdmitted, stage.lookupIntervalLocallyAdmitted,
		stage.censusIntervalLocallyAdmitted, stage.supersedeCPUUnavailable,
		stage.selectCPUKnownCallstack, stage.supersedeCPUKnownCallstack,
		stage.upsertForced, stage.insertFence,
	} {
		if statement != nil {
			errs = append(errs, statement.Close())
		}
	}
	stage.insertCandidate, stage.insertIdentity, stage.lookupIdentity = nil, nil, nil
	stage.lookupSemantic, stage.lookupInterval = nil, nil
	stage.lookupSemanticLocallyAdmitted, stage.lookupIntervalLocallyAdmitted = nil, nil
	stage.censusIntervalLocallyAdmitted = nil
	stage.supersedeCPUUnavailable = nil
	stage.selectCPUKnownCallstack, stage.supersedeCPUKnownCallstack = nil, nil
	stage.upsertForced, stage.insertFence = nil, nil
	return errors.Join(errs...)
}

func (stage *traceDBSyncSpanStage) verifySQLiteLanePlan(ctx context.Context) error {
	if stage.conn == nil {
		return &traceDBOutputInvariantError{Reason: "sync_span_stage_missing_read_connection"}
	}
	rows, err := stage.conn.QueryContext(ctx, "EXPLAIN QUERY PLAN "+traceDBSyncSpanSelectCandidatesSQL)
	if err != nil {
		return fmt.Errorf("explain sync span lane iterator: %w", err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int64
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			return err
		}
		details = append(details, strings.ToUpper(detail))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	plan := strings.Join(details, "\n")
	if !strings.Contains(plan, "CANDIDATE_LANE_IDX") || strings.Contains(plan, "TEMP B-TREE") {
		return &traceDBOutputInvariantError{Reason: "sync_span_stage_unbounded_lane_query_plan"}
	}
	rows, err = stage.conn.QueryContext(ctx, "EXPLAIN QUERY PLAN "+traceDBSyncSpanSelectFencesSQL)
	if err != nil {
		return fmt.Errorf("explain sync span fence iterator: %w", err)
	}
	defer rows.Close()
	details = details[:0]
	for rows.Next() {
		var id, parent, unused int64
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			return err
		}
		details = append(details, strings.ToUpper(detail))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	plan = strings.Join(details, "\n")
	if !strings.Contains(plan, "FENCE_LANE_IDX") || strings.Contains(plan, "TEMP B-TREE") {
		return &traceDBOutputInvariantError{Reason: "sync_span_stage_unbounded_fence_query_plan"}
	}
	stage.stats.LanePlanVerified = true
	return nil
}

func (stage *traceDBSyncSpanStage) requireSealed(ctx context.Context) error {
	if stage == nil || stage.closed || !stage.sealed {
		return &traceDBOutputInvariantError{Reason: "sync_span_stage_not_sealed"}
	}
	if ctx == nil {
		return &traceDBOutputInvariantError{Reason: "missing_sync_span_stage_context"}
	}
	return ctx.Err()
}

type traceDBSyncSpanCandidateIterator interface {
	next(context.Context) (traceDBSyncSpanStagedCandidate, bool, error)
	close() error
}

func (stage *traceDBSyncSpanStage) candidateIterator(ctx context.Context) (traceDBSyncSpanCandidateIterator, error) {
	if err := stage.requireSealed(ctx); err != nil {
		return nil, err
	}
	if stage.external {
		rows, err := stage.conn.QueryContext(ctx, traceDBSyncSpanSelectCandidatesSQL)
		if err != nil {
			return nil, fmt.Errorf("open sync span candidate iterator: %w", err)
		}
		return &traceDBSyncSpanSQLiteCandidateIterator{rows: rows}, nil
	}
	return &traceDBSyncSpanMemoryCandidateIterator{items: stage.memoryCandidates}, nil
}

type traceDBSyncSpanMemoryCandidateIterator struct {
	items []traceDBSyncSpanStagedCandidate
	index int
}

func (iterator *traceDBSyncSpanMemoryCandidateIterator) next(ctx context.Context) (traceDBSyncSpanStagedCandidate, bool, error) {
	if err := ctx.Err(); err != nil {
		return traceDBSyncSpanStagedCandidate{}, false, err
	}
	if iterator.index >= len(iterator.items) {
		return traceDBSyncSpanStagedCandidate{}, false, nil
	}
	item := iterator.items[iterator.index]
	iterator.index++
	if item.Ordinal <= 0 {
		return traceDBSyncSpanStagedCandidate{}, false, &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_ordinal"}
	}
	if err := validateTraceDBSyncSpanCandidate(item.Candidate); err != nil {
		return traceDBSyncSpanStagedCandidate{}, false, err
	}
	return item, true, nil
}

func (*traceDBSyncSpanMemoryCandidateIterator) close() error { return nil }

type traceDBSyncSpanSQLiteCandidateIterator struct {
	rows *sql.Rows
}

func (iterator *traceDBSyncSpanSQLiteCandidateIterator) next(ctx context.Context) (traceDBSyncSpanStagedCandidate, bool, error) {
	if err := ctx.Err(); err != nil {
		return traceDBSyncSpanStagedCandidate{}, false, err
	}
	if !iterator.rows.Next() {
		if err := iterator.rows.Err(); err != nil {
			return traceDBSyncSpanStagedCandidate{}, false, err
		}
		return traceDBSyncSpanStagedCandidate{}, false, nil
	}
	var item traceDBSyncSpanStagedCandidate
	var key []byte
	var zeroKey, producer, stableKind int64
	var markerKnown, canonicalKnown, ownerKnown, cpuPlacement, startCPUProvenance, endCPUProvenance int64
	var nameProvenance, depthKnown, depthProvenance int64
	candidate := &item.Candidate
	if err := iterator.rows.Scan(
		&item.Ordinal, &key, &zeroKey, &producer, &stableKind, &candidate.StableID,
		&candidate.HeaderTID, &candidate.HeaderTGID, &candidate.MarkerPID, &markerKnown,
		&candidate.CanonicalITID, &canonicalKnown,
		&candidate.OwnerIPID, &ownerKnown, &candidate.Start, &candidate.End,
		&candidate.StartCPU, &candidate.EndCPU,
		&candidate.StartFlags, &candidate.EndFlags,
		&candidate.StartPreemptCount, &candidate.EndPreemptCount,
		&candidate.StartMarkerBody, &candidate.EndMarkerBody,
		&cpuPlacement, &startCPUProvenance, &endCPUProvenance,
		&candidate.Task, &candidate.Name, &nameProvenance, &candidate.Depth, &depthKnown, &depthProvenance,
	); err != nil {
		return traceDBSyncSpanStagedCandidate{}, false, err
	}
	if item.Ordinal <= 0 || producer <= int64(traceDBSyncSpanProducerUnknown) || producer > int64(traceDBSyncSpanProducerSourceRawMarker) ||
		stableKind <= int64(traceDBSyncSpanStableUnknown) || stableKind > int64(traceDBSyncSpanStableSourceRawOrdinal) ||
		cpuPlacement < int64(traceDBSyncSpanCPUPlacementKnown) || cpuPlacement > int64(traceDBSyncSpanCPUPlacementAliasAmbiguous) ||
		startCPUProvenance < int64(traceDBSyncSpanCPUUnknown) || startCPUProvenance > int64(traceDBSyncSpanCPUSourceRawPage) ||
		endCPUProvenance < int64(traceDBSyncSpanCPUUnknown) || endCPUProvenance > int64(traceDBSyncSpanCPUSourceRawPage) ||
		nameProvenance <= int64(traceDBSyncSpanNameUnknown) || nameProvenance > int64(traceDBSyncSpanNameSourceRawMarker) ||
		depthProvenance < int64(traceDBSyncSpanDepthUnknown) || depthProvenance > int64(traceDBSyncSpanDepthCallstack) {
		return traceDBSyncSpanStagedCandidate{}, false, &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_enum"}
	}
	var ok bool
	if candidate.MarkerPIDKnown, ok = traceDBSQLiteExactBool(markerKnown); !ok {
		return traceDBSyncSpanStagedCandidate{}, false, &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_boolean"}
	}
	if candidate.CanonicalITIDKnown, ok = traceDBSQLiteExactBool(canonicalKnown); !ok {
		return traceDBSyncSpanStagedCandidate{}, false, &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_boolean"}
	}
	if candidate.OwnerIPIDKnown, ok = traceDBSQLiteExactBool(ownerKnown); !ok {
		return traceDBSyncSpanStagedCandidate{}, false, &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_boolean"}
	}
	if candidate.DepthKnown, ok = traceDBSQLiteExactBool(depthKnown); !ok {
		return traceDBSyncSpanStagedCandidate{}, false, &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_boolean"}
	}
	candidate.Producer = traceDBSyncSpanProducer(producer)
	candidate.StableKind = traceDBSyncSpanStableKind(stableKind)
	candidate.CPUPlacement = traceDBSyncSpanCPUPlacement(cpuPlacement)
	candidate.StartCPUProvenance = traceDBSyncSpanCPUProvenance(startCPUProvenance)
	candidate.EndCPUProvenance = traceDBSyncSpanCPUProvenance(endCPUProvenance)
	candidate.NameProvenance = traceDBSyncSpanNameProvenance(nameProvenance)
	candidate.DepthProvenance = traceDBSyncSpanDepthProvenance(depthProvenance)
	expectedZeroKey := boolToSQLiteInt(candidate.Start != candidate.End)
	if zeroKey != expectedZeroKey || !bytes.Equal(key, traceDBSyncSpanLaneOrderKey(*candidate)) {
		return traceDBSyncSpanStagedCandidate{}, false, &traceDBOutputInvariantError{Reason: "sync_span_stage_order_key_mismatch"}
	}
	if err := validateTraceDBSyncSpanCandidate(*candidate); err != nil {
		return traceDBSyncSpanStagedCandidate{}, false, err
	}
	return item, true, nil
}

func (iterator *traceDBSyncSpanSQLiteCandidateIterator) close() error {
	if iterator == nil || iterator.rows == nil {
		return nil
	}
	err := iterator.rows.Close()
	iterator.rows = nil
	return err
}

func traceDBSQLiteExactBool(value int64) (bool, bool) {
	switch value {
	case 0:
		return false, true
	case 1:
		return true, true
	default:
		return false, false
	}
}

type traceDBSyncSpanFenceIterator interface {
	next(context.Context) (traceDBSyncSpanStagedFence, bool, error)
	close() error
}

func (stage *traceDBSyncSpanStage) fenceIterator(ctx context.Context) (traceDBSyncSpanFenceIterator, error) {
	if err := stage.requireSealed(ctx); err != nil {
		return nil, err
	}
	if stage.external {
		rows, err := stage.conn.QueryContext(ctx, traceDBSyncSpanSelectFencesSQL)
		if err != nil {
			return nil, fmt.Errorf("open sync span fence iterator: %w", err)
		}
		return &traceDBSyncSpanSQLiteFenceIterator{rows: rows}, nil
	}
	return &traceDBSyncSpanMemoryFenceIterator{items: stage.memoryFences}, nil
}

type traceDBSyncSpanMemoryFenceIterator struct {
	items []traceDBSyncSpanStagedFence
	index int
}

func (iterator *traceDBSyncSpanMemoryFenceIterator) next(ctx context.Context) (traceDBSyncSpanStagedFence, bool, error) {
	if err := ctx.Err(); err != nil {
		return traceDBSyncSpanStagedFence{}, false, err
	}
	if iterator.index >= len(iterator.items) {
		return traceDBSyncSpanStagedFence{}, false, nil
	}
	item := iterator.items[iterator.index]
	iterator.index++
	if item.Ordinal <= 0 {
		return traceDBSyncSpanStagedFence{}, false, &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_fence_ordinal"}
	}
	if err := validateTraceDBSyncSpanLaneFence(item.Fence); err != nil {
		return traceDBSyncSpanStagedFence{}, false, err
	}
	return item, true, nil
}

func (*traceDBSyncSpanMemoryFenceIterator) close() error { return nil }

type traceDBSyncSpanSQLiteFenceIterator struct {
	rows *sql.Rows
}

func (iterator *traceDBSyncSpanSQLiteFenceIterator) next(ctx context.Context) (traceDBSyncSpanStagedFence, bool, error) {
	if err := ctx.Err(); err != nil {
		return traceDBSyncSpanStagedFence{}, false, err
	}
	if !iterator.rows.Next() {
		if err := iterator.rows.Err(); err != nil {
			return traceDBSyncSpanStagedFence{}, false, err
		}
		return traceDBSyncSpanStagedFence{}, false, nil
	}
	var item traceDBSyncSpanStagedFence
	var producer, kind, reason int64
	if err := iterator.rows.Scan(
		&item.Ordinal, &producer, &item.Fence.HeaderTID, &item.Fence.CanonicalITID,
		&item.Fence.Start, &item.Fence.End, &kind, &reason,
	); err != nil {
		return traceDBSyncSpanStagedFence{}, false, err
	}
	if item.Ordinal <= 0 ||
		producer <= int64(traceDBSyncSpanProducerUnknown) || producer > int64(traceDBSyncSpanProducerSourceRawMarker) ||
		kind <= int64(traceDBSyncSpanFenceUnknown) || kind > int64(traceDBSyncSpanFenceSuffix) ||
		reason <= int64(traceDBSyncSpanLanePoisonUnknown) || reason > int64(traceDBSyncSpanLanePoisonRejectedSyscallCandidate) {
		return traceDBSyncSpanStagedFence{}, false, &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_fence_enum"}
	}
	item.Fence.Producer = traceDBSyncSpanProducer(producer)
	item.Fence.CanonicalITIDKnown = true
	item.Fence.Kind = traceDBSyncSpanFenceKind(kind)
	item.Fence.Reason = traceDBSyncSpanLanePoisonReason(reason)
	if err := validateTraceDBSyncSpanLaneFence(item.Fence); err != nil {
		return traceDBSyncSpanStagedFence{}, false, err
	}
	return item, true, nil
}

func (iterator *traceDBSyncSpanSQLiteFenceIterator) close() error {
	if iterator == nil || iterator.rows == nil {
		return nil
	}
	err := iterator.rows.Close()
	iterator.rows = nil
	return err
}

type traceDBSyncSpanForcedLane struct {
	HeaderTID int64
	Reason    traceDBSyncSpanForcedReason
}

type traceDBSyncSpanForcedIterator interface {
	next(context.Context) (traceDBSyncSpanForcedLane, bool, error)
	close() error
}

func (stage *traceDBSyncSpanStage) forcedIterator(ctx context.Context) (traceDBSyncSpanForcedIterator, error) {
	if err := stage.requireSealed(ctx); err != nil {
		return nil, err
	}
	if stage.external {
		rows, err := stage.conn.QueryContext(ctx, traceDBSyncSpanSelectForcedSQL)
		if err != nil {
			return nil, fmt.Errorf("open sync span forced-lane iterator: %w", err)
		}
		return &traceDBSyncSpanSQLiteForcedIterator{rows: rows}, nil
	}
	indexBytes := int64(len(stage.memoryForced)) * 8
	if stage.residentBytes > stage.options.ResidentBytes-indexBytes {
		return nil, &traceDBOutputInvariantError{Reason: "sync_span_stage_forced_index_budget_drift"}
	}
	if peak := stage.residentBytes + indexBytes; peak > stage.stats.PeakResidentBytes {
		stage.stats.PeakResidentBytes = peak
	}
	tids := make([]int64, 0, len(stage.memoryForced))
	for tid := range stage.memoryForced {
		tids = append(tids, tid)
	}
	sort.Slice(tids, func(i, j int) bool { return tids[i] < tids[j] })
	return &traceDBSyncSpanMemoryForcedIterator{tids: tids, reasons: stage.memoryForced}, nil
}

type traceDBSyncSpanMemoryForcedIterator struct {
	tids    []int64
	reasons map[int64]traceDBSyncSpanForcedReason
	index   int
}

func (iterator *traceDBSyncSpanMemoryForcedIterator) next(ctx context.Context) (traceDBSyncSpanForcedLane, bool, error) {
	if err := ctx.Err(); err != nil {
		return traceDBSyncSpanForcedLane{}, false, err
	}
	if iterator.index >= len(iterator.tids) {
		return traceDBSyncSpanForcedLane{}, false, nil
	}
	tid := iterator.tids[iterator.index]
	iterator.index++
	item := traceDBSyncSpanForcedLane{HeaderTID: tid, Reason: iterator.reasons[tid]}
	if err := validateTraceDBSyncSpanForcedLane(item); err != nil {
		return traceDBSyncSpanForcedLane{}, false, err
	}
	return item, true, nil
}

func (*traceDBSyncSpanMemoryForcedIterator) close() error { return nil }

type traceDBSyncSpanSQLiteForcedIterator struct {
	rows *sql.Rows
}

func (iterator *traceDBSyncSpanSQLiteForcedIterator) next(ctx context.Context) (traceDBSyncSpanForcedLane, bool, error) {
	if err := ctx.Err(); err != nil {
		return traceDBSyncSpanForcedLane{}, false, err
	}
	if !iterator.rows.Next() {
		if err := iterator.rows.Err(); err != nil {
			return traceDBSyncSpanForcedLane{}, false, err
		}
		return traceDBSyncSpanForcedLane{}, false, nil
	}
	var item traceDBSyncSpanForcedLane
	var reason int64
	if err := iterator.rows.Scan(&item.HeaderTID, &reason); err != nil {
		return traceDBSyncSpanForcedLane{}, false, err
	}
	if reason <= 0 || reason > int64(traceDBSyncSpanForcedAll) {
		return traceDBSyncSpanForcedLane{}, false, &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_forced_reason"}
	}
	item.Reason = traceDBSyncSpanForcedReason(reason)
	if err := validateTraceDBSyncSpanForcedLane(item); err != nil {
		return traceDBSyncSpanForcedLane{}, false, err
	}
	return item, true, nil
}

func (iterator *traceDBSyncSpanSQLiteForcedIterator) close() error {
	if iterator == nil || iterator.rows == nil {
		return nil
	}
	err := iterator.rows.Close()
	iterator.rows = nil
	return err
}

func validateTraceDBSyncSpanForcedLane(item traceDBSyncSpanForcedLane) error {
	if item.HeaderTID < 0 || item.HeaderTID > math.MaxInt32 || item.Reason == traceDBSyncSpanForcedNone || item.Reason&^traceDBSyncSpanForcedAll != 0 ||
		(item.HeaderTID == 0 && item.Reason&traceDBSyncSpanForcedPoison != 0) {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_stage_forced_lane"}
	}
	return nil
}

func (stage *traceDBSyncSpanStage) budget() string {
	if stage == nil {
		return ""
	}
	return stage.budgetReason
}

func (stage *traceDBSyncSpanStage) snapshotStats() traceDBSyncSpanStageStats {
	if stage == nil {
		return traceDBSyncSpanStageStats{}
	}
	return stage.stats
}

func (stage *traceDBSyncSpanStage) noteAuditComparison() error {
	if stage.stats.AuditComparisons >= stage.options.MaxAuditComparisons {
		return stage.failBudget(traceDBSyncSpanStageBudgetAuditCompareCap)
	}
	stage.stats.AuditComparisons++
	return nil
}

func (stage *traceDBSyncSpanStage) noteActive(depth int, bytes int64) error {
	if depth > stage.options.MaxActiveDepth {
		return stage.failBudget(traceDBSyncSpanStageBudgetActiveDepthCap)
	}
	if bytes > stage.options.MaxActiveBytes {
		return stage.failBudget(traceDBSyncSpanStageBudgetActiveByteCap)
	}
	if depth > stage.stats.PeakActiveDepth {
		stage.stats.PeakActiveDepth = depth
	}
	if bytes > stage.stats.PeakActiveBytes {
		stage.stats.PeakActiveBytes = bytes
	}
	return nil
}

func (stage *traceDBSyncSpanStage) cleanup() error {
	if stage == nil {
		return nil
	}
	if stage.closed {
		return stage.cleanupErr
	}
	err := stage.discardBackend()
	stage.memoryCandidates = nil
	stage.memoryFences = nil
	stage.memoryIdentityFirst = nil
	stage.memoryForced = nil
	stage.residentBytes = 0
	workspace := stage.workspace
	if workspace != "" {
		if removeErr := os.RemoveAll(workspace); removeErr != nil {
			err = errors.Join(err, removeErr)
		} else {
			stage.workspace = ""
		}
	}
	stage.cleanupErr = errors.Join(stage.cleanupErr, err)
	if stage.workspace == "" {
		stage.closed = true
	}
	return stage.cleanupErr
}

const (
	traceDBSyncSpanBadLaneHeaderBytes = 16
	traceDBSyncSpanBadLaneFooterBytes = 16
	traceDBSyncSpanBadLaneRecordBytes = 8
)

var (
	traceDBSyncSpanBadLaneMagic  = [8]byte{'C', 'D', 'X', 'B', 'A', 'D', 'S', '1'}
	traceDBSyncSpanBadLaneFooter = [4]byte{'E', 'N', 'D', '1'}
)

type traceDBSyncSpanBadLaneJournal struct {
	stage   *traceDBSyncSpanStage
	path    string
	file    *os.File
	buffer  *bufio.Writer
	crc     hash32
	count   uint64
	lastTID int64
	sealed  bool
}

type hash32 interface {
	Write([]byte) (int, error)
	Sum32() uint32
}

func (stage *traceDBSyncSpanStage) newBadLaneJournal() (*traceDBSyncSpanBadLaneJournal, error) {
	if stage == nil || stage.closed || !stage.sealed {
		return nil, &traceDBOutputInvariantError{Reason: "sync_span_bad_lane_journal_stage_state"}
	}
	return &traceDBSyncSpanBadLaneJournal{stage: stage, crc: crc32.NewIEEE()}, nil
}

func (journal *traceDBSyncSpanBadLaneJournal) add(ctx context.Context, tid int64) error {
	if journal == nil || journal.stage == nil || journal.sealed || tid < 0 || tid > math.MaxInt32 ||
		(journal.count > 0 && tid <= journal.lastTID) {
		return &traceDBOutputInvariantError{Reason: "invalid_sync_span_bad_lane_journal_order"}
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, journal.abort())
	}
	if journal.file == nil {
		path := filepath.Join(journal.stage.workspace, "bad-lanes.bin")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("create sync span bad-lane journal: %w", err)
		}
		journal.path = path
		journal.file = file
		journal.buffer = bufio.NewWriterSize(file, 64*1024)
		if err := journal.stage.reserveTempBytes(traceDBSyncSpanBadLaneHeaderBytes); err != nil {
			return errors.Join(err, journal.abort())
		}
		var header [traceDBSyncSpanBadLaneHeaderBytes]byte
		copy(header[:8], traceDBSyncSpanBadLaneMagic[:])
		binary.BigEndian.PutUint32(header[8:12], 1)
		if _, err := journal.buffer.Write(header[:]); err != nil {
			return errors.Join(fmt.Errorf("write sync span bad-lane header: %w", err), journal.abort())
		}
	}
	if err := journal.stage.reserveTempBytes(traceDBSyncSpanBadLaneRecordBytes); err != nil {
		return errors.Join(err, journal.abort())
	}
	var raw [traceDBSyncSpanBadLaneRecordBytes]byte
	binary.BigEndian.PutUint64(raw[:], uint64(tid))
	if _, err := journal.buffer.Write(raw[:]); err != nil {
		return errors.Join(fmt.Errorf("write sync span bad-lane record: %w", err), journal.abort())
	}
	if _, err := journal.crc.Write(raw[:]); err != nil {
		return errors.Join(fmt.Errorf("checksum sync span bad-lane record: %w", err), journal.abort())
	}
	journal.count++
	journal.lastTID = tid
	return nil
}

func (journal *traceDBSyncSpanBadLaneJournal) seal(ctx context.Context) error {
	if journal == nil || journal.stage == nil || journal.sealed {
		return &traceDBOutputInvariantError{Reason: "sync_span_bad_lane_journal_seal_state"}
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, journal.abort())
	}
	if journal.file == nil {
		journal.sealed = true
		return nil
	}
	if err := journal.stage.reserveTempBytes(traceDBSyncSpanBadLaneFooterBytes); err != nil {
		return errors.Join(err, journal.abort())
	}
	var footer [traceDBSyncSpanBadLaneFooterBytes]byte
	copy(footer[:4], traceDBSyncSpanBadLaneFooter[:])
	binary.BigEndian.PutUint64(footer[4:12], journal.count)
	binary.BigEndian.PutUint32(footer[12:16], journal.crc.Sum32())
	if _, err := journal.buffer.Write(footer[:]); err != nil {
		return errors.Join(fmt.Errorf("write sync span bad-lane footer: %w", err), journal.abort())
	}
	if err := journal.buffer.Flush(); err != nil {
		return errors.Join(fmt.Errorf("flush sync span bad-lane journal: %w", err), journal.abort())
	}
	if err := journal.file.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync sync-span bad-lane journal: %w", err), journal.abort())
	}
	if err := journal.file.Close(); err != nil {
		return errors.Join(fmt.Errorf("close sync span bad-lane journal: %w", err), journal.abort())
	}
	journal.buffer = nil
	journal.file = nil
	if err := journal.stage.sampleTempBytes(); err != nil {
		return errors.Join(err, journal.abort())
	}
	journal.stage.stats.ExternalArtifacts++
	journal.sealed = true
	return nil
}

func (journal *traceDBSyncSpanBadLaneJournal) abort() error {
	if journal == nil {
		return nil
	}
	var err error
	if journal.file != nil {
		err = journal.file.Close()
		journal.file = nil
	}
	journal.buffer = nil
	if journal.path != "" {
		if removeErr := os.Remove(journal.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}
	return err
}

func (journal *traceDBSyncSpanBadLaneJournal) reader(ctx context.Context) (*traceDBSyncSpanBadLaneReader, error) {
	if journal == nil || !journal.sealed {
		return nil, &traceDBOutputInvariantError{Reason: "sync_span_bad_lane_journal_not_sealed"}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if journal.path == "" {
		return &traceDBSyncSpanBadLaneReader{empty: true}, nil
	}
	return openTraceDBSyncSpanBadLaneReader(ctx, journal.path)
}

type traceDBSyncSpanBadLaneReader struct {
	file        *os.File
	crc         hash32
	count       uint64
	index       uint64
	expectedCRC uint32
	lastTID     int64
	empty       bool
	verified    bool
}

func openTraceDBSyncSpanBadLaneReader(ctx context.Context, path string) (*traceDBSyncSpanBadLaneReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fail := func(reason string, cause error) (*traceDBSyncSpanBadLaneReader, error) {
		return nil, errors.Join(&traceDBOutputInvariantError{Reason: reason}, cause, file.Close())
	}
	info, err := file.Stat()
	if err != nil {
		return fail("sync_span_bad_lane_journal_stat", err)
	}
	if info.Size() < traceDBSyncSpanBadLaneHeaderBytes+traceDBSyncSpanBadLaneFooterBytes {
		return fail("sync_span_bad_lane_journal_truncated", nil)
	}
	var header [traceDBSyncSpanBadLaneHeaderBytes]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return fail("sync_span_bad_lane_journal_header", err)
	}
	if !bytes.Equal(header[:8], traceDBSyncSpanBadLaneMagic[:]) || binary.BigEndian.Uint32(header[8:12]) != 1 || binary.BigEndian.Uint32(header[12:16]) != 0 {
		return fail("sync_span_bad_lane_journal_header", nil)
	}
	var footer [traceDBSyncSpanBadLaneFooterBytes]byte
	if _, err := file.ReadAt(footer[:], info.Size()-traceDBSyncSpanBadLaneFooterBytes); err != nil {
		return fail("sync_span_bad_lane_journal_footer", err)
	}
	if !bytes.Equal(footer[:4], traceDBSyncSpanBadLaneFooter[:]) {
		return fail("sync_span_bad_lane_journal_footer", nil)
	}
	count := binary.BigEndian.Uint64(footer[4:12])
	if count == 0 || count > uint64(math.MaxInt64/traceDBSyncSpanBadLaneRecordBytes) {
		return fail("sync_span_bad_lane_journal_count", nil)
	}
	expectedSize := int64(traceDBSyncSpanBadLaneHeaderBytes+traceDBSyncSpanBadLaneFooterBytes) + int64(count)*traceDBSyncSpanBadLaneRecordBytes
	if expectedSize != info.Size() {
		return fail("sync_span_bad_lane_journal_size", nil)
	}
	if _, err := file.Seek(traceDBSyncSpanBadLaneHeaderBytes, io.SeekStart); err != nil {
		return fail("sync_span_bad_lane_journal_seek", err)
	}
	verificationCRC := crc32.NewIEEE()
	var lastTID int64
	for index := uint64(0); index < count; index++ {
		if index&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return fail("sync_span_bad_lane_journal_canceled", err)
			}
		}
		var raw [traceDBSyncSpanBadLaneRecordBytes]byte
		if _, err := io.ReadFull(file, raw[:]); err != nil {
			return fail("sync_span_bad_lane_journal_record", err)
		}
		_, _ = verificationCRC.Write(raw[:])
		value := binary.BigEndian.Uint64(raw[:])
		if value > math.MaxInt32 || (index > 0 && int64(value) <= lastTID) {
			return fail("sync_span_bad_lane_journal_order", nil)
		}
		lastTID = int64(value)
	}
	if verificationCRC.Sum32() != binary.BigEndian.Uint32(footer[12:16]) {
		return fail("sync_span_bad_lane_journal_checksum", nil)
	}
	if _, err := file.Seek(traceDBSyncSpanBadLaneHeaderBytes, io.SeekStart); err != nil {
		return fail("sync_span_bad_lane_journal_seek", err)
	}
	return &traceDBSyncSpanBadLaneReader{
		file: file, crc: crc32.NewIEEE(), count: count, expectedCRC: binary.BigEndian.Uint32(footer[12:16]),
	}, nil
}

func (reader *traceDBSyncSpanBadLaneReader) next(ctx context.Context) (int64, bool, error) {
	if reader == nil {
		return 0, false, &traceDBOutputInvariantError{Reason: "missing_sync_span_bad_lane_reader"}
	}
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	if reader.empty {
		reader.verified = true
		return 0, false, nil
	}
	if reader.index == reader.count {
		if !reader.verified {
			if reader.crc.Sum32() != reader.expectedCRC {
				return 0, false, &traceDBOutputInvariantError{Reason: "sync_span_bad_lane_journal_checksum"}
			}
			reader.verified = true
		}
		return 0, false, nil
	}
	var raw [traceDBSyncSpanBadLaneRecordBytes]byte
	if _, err := io.ReadFull(reader.file, raw[:]); err != nil {
		return 0, false, err
	}
	if _, err := reader.crc.Write(raw[:]); err != nil {
		return 0, false, err
	}
	value := binary.BigEndian.Uint64(raw[:])
	if value > math.MaxInt32 {
		return 0, false, &traceDBOutputInvariantError{Reason: "sync_span_bad_lane_journal_tid"}
	}
	tid := int64(value)
	if reader.index > 0 && tid <= reader.lastTID {
		return 0, false, &traceDBOutputInvariantError{Reason: "sync_span_bad_lane_journal_order"}
	}
	reader.index++
	reader.lastTID = tid
	return tid, true, nil
}

func (reader *traceDBSyncSpanBadLaneReader) close() error {
	if reader == nil || reader.file == nil {
		return nil
	}
	err := reader.file.Close()
	reader.file = nil
	return err
}
