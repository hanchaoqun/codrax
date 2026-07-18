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
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const (
	defaultTraceDBRawPairingMaxRecords       int64 = 4_000_000
	defaultTraceDBRawPairingMaxLaneKeys      int64 = 1_000_000
	defaultTraceDBRawPairingMaxTempBytes     int64 = 4 << 30
	defaultTraceDBRawPairingSQLiteCacheKiB         = 16 << 10
	traceDBRawPairingSQLitePageBytes               = 4096
	traceDBRawPairingSQLiteMinimumPages            = 32
	traceDBRawPairingSQLiteWriteReservePages       = 16
)

const (
	traceDBRawPairingBudgetRecordCap     = "record_cap"
	traceDBRawPairingBudgetLaneKeyCap    = "lane_key_cap"
	traceDBRawPairingBudgetSQLitePageCap = "sqlite_page_cap"
	traceDBRawPairingBudgetTempByteCap   = "temp_byte_cap"
	traceDBRawPairingBudgetSequenceCap   = "endpoint_sequence_cap"
)

type traceDBRawPairingStageOptions struct {
	TempRoot     string
	MaxRecords   int64
	MaxLaneKeys  int64
	MaxTempBytes int64
	SQLiteCache  int
}

func normalizedTraceDBRawPairingStageOptions(options traceDBRawPairingStageOptions) traceDBRawPairingStageOptions {
	if options.MaxRecords <= 0 {
		options.MaxRecords = defaultTraceDBRawPairingMaxRecords
	}
	if options.MaxLaneKeys <= 0 {
		options.MaxLaneKeys = defaultTraceDBRawPairingMaxLaneKeys
	}
	if options.MaxTempBytes <= 0 {
		options.MaxTempBytes = defaultTraceDBRawPairingMaxTempBytes
	}
	if options.SQLiteCache <= 0 {
		options.SQLiteCache = defaultTraceDBRawPairingSQLiteCacheKiB
	}
	return options
}

type traceDBRawPairingStageState uint8

const (
	traceDBRawPairingStageOpen traceDBRawPairingStageState = iota
	traceDBRawPairingStageSealed
	traceDBRawPairingStagePublished
	traceDBRawPairingStageFailed
)

type traceDBRawPairingStageBudgetError struct{ Reason string }

func (err *traceDBRawPairingStageBudgetError) Error() string {
	return "trace DB raw pairing stage budget exceeded: " + err.Reason
}

func traceDBRawPairingStageBudgetReason(err error) (string, bool) {
	var budget *traceDBRawPairingStageBudgetError
	if !errors.As(err, &budget) {
		return "", false
	}
	return budget.Reason, true
}

// traceDBRawPairingObservation is the complete pass-1 disposition of one raw
// row. Every physical row consumes the bounded stage record budget, including
// unsupported and rejected rows. Only rows that can affect publication or a
// governed pairing lane need a payload record in the private database.
type traceDBRawPairingObservation struct {
	StableID       int64
	StableKnown    bool
	Timestamp      int64
	TimestampKnown bool
	Class          string
	Line           string
	Publishable    bool

	Verdict            tracequery.PairingEndpointVerdict
	LaneKey            string
	HeaderOwnerKnown   bool
	CanonicalITID      int64
	CanonicalITIDKnown bool
	EndpointAdmitted   bool
}

type traceDBRawPairingStageReport struct {
	PhysicalRows          int64
	StoredRows            int64
	PublishedRows         int64
	DuplicatePhysicalRows int64
	PoisonedLanes         int64
	PoisonedFamilies      int64
	PeakTempBytes         int64
	BudgetReason          string
	SuppressedRows        int64
}

type traceDBRawPairingStage struct {
	options        traceDBRawPairingStageOptions
	artifactSource string
	workspace      string
	dbPath         string
	db             *sql.DB
	conn           *sql.Conn
	tx             *sql.Tx
	state          traceDBRawPairingStageState
	closed         bool
	cleanupErr     error
	budgetReason   string
	baseMaxPages   int64
	maxPages       int64
	records        int64
	stored         int64
	laneCounts     map[tracequery.PairingEndpointFamily]int64
	familyPoisoned map[tracequery.PairingEndpointFamily]bool
	peakTempBytes  int64

	insertRecord       *sql.Stmt
	upsertStable       *sql.Stmt
	insertDuplicate    *sql.Stmt
	insertLaneSeen     *sql.Stmt
	insertPoisonLane   *sql.Stmt
	insertPoisonFamily *sql.Stmt
}

var traceDBRawPairingStageSchema = []string{
	`CREATE TABLE record (
		ordinal INTEGER PRIMARY KEY,
		stable_id INTEGER NOT NULL,
		stable_known INTEGER NOT NULL CHECK(stable_known IN (0,1)),
		stable_order INTEGER NOT NULL,
		ts_ns INTEGER NOT NULL,
		class TEXT NOT NULL,
		line TEXT NOT NULL,
		publishable INTEGER NOT NULL CHECK(publishable IN (0,1)),
		endpoint INTEGER NOT NULL CHECK(endpoint IN (0,1)),
		family TEXT NOT NULL,
		phase TEXT NOT NULL,
		lane_key BLOB,
		owner_known INTEGER NOT NULL CHECK(owner_known IN (0,1)),
		canonical_itid INTEGER NOT NULL,
		canonical_known INTEGER NOT NULL CHECK(canonical_known IN (0,1))
	)`,
	`CREATE INDEX raw_record_publish_idx ON record(ts_ns,stable_order,ordinal)`,
	`CREATE INDEX raw_record_lane_audit_idx ON record(family,lane_key,stable_order,ordinal)
		WHERE endpoint=1 AND publishable=1`,
	`CREATE TABLE stable_identity (
		stable_id INTEGER PRIMARY KEY,
		occurrences INTEGER NOT NULL CHECK(occurrences > 0)
	) WITHOUT ROWID`,
	`CREATE TABLE duplicate_stable (
		stable_id INTEGER PRIMARY KEY
	) WITHOUT ROWID`,
	`CREATE TABLE lane_seen (
		family TEXT NOT NULL,
		lane_key BLOB NOT NULL,
		PRIMARY KEY(family,lane_key)
	) WITHOUT ROWID`,
	`CREATE TABLE poison_lane (
		family TEXT NOT NULL,
		lane_key BLOB NOT NULL,
		PRIMARY KEY(family,lane_key)
	) WITHOUT ROWID`,
	`CREATE TABLE poison_family (
		family TEXT PRIMARY KEY
	) WITHOUT ROWID`,
}

const traceDBRawPairingInsertRecordSQL = `INSERT INTO record(
	ordinal,stable_id,stable_known,stable_order,ts_ns,class,line,publishable,
	endpoint,family,phase,lane_key,owner_known,canonical_itid,canonical_known
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

const traceDBRawPairingPublishWhere = `
	r.publishable=1 AND d.stable_id IS NULL AND
	(r.endpoint=0 OR (pf.family IS NULL AND pl.family IS NULL))`

const traceDBRawPairingPublishFrom = `
	FROM record AS r INDEXED BY raw_record_publish_idx
	LEFT JOIN duplicate_stable AS d ON r.stable_known=1 AND d.stable_id=r.stable_id
	LEFT JOIN poison_family AS pf ON r.endpoint=1 AND pf.family=r.family
	LEFT JOIN poison_lane AS pl ON r.endpoint=1 AND pl.family=r.family AND pl.lane_key=r.lane_key`

func newTraceDBRawPairingStage(ctx context.Context, artifactSource string, options traceDBRawPairingStageOptions) (*traceDBRawPairingStage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	options = normalizedTraceDBRawPairingStageOptions(options)
	artifactSource = strings.TrimSpace(artifactSource)
	if artifactSource == "" || options.MaxRecords <= 0 || options.MaxLaneKeys <= 0 ||
		options.MaxTempBytes < traceDBRawPairingSQLitePageBytes || options.MaxRecords > math.MaxInt64-1 {
		return nil, &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_stage_options"}
	}
	abs, err := filepath.Abs(artifactSource)
	if err != nil {
		return nil, fmt.Errorf("resolve raw pairing artifact source: %w", err)
	}
	workspace, err := os.MkdirTemp(options.TempRoot, "codrax-tracedb-raw-pairing-*")
	if err != nil {
		return nil, fmt.Errorf("create raw pairing stage workspace: %w", err)
	}
	if err := os.Chmod(workspace, 0o700); err != nil {
		return nil, errors.Join(fmt.Errorf("secure raw pairing stage workspace: %w", err), os.RemoveAll(workspace))
	}
	stage := &traceDBRawPairingStage{
		options: options, artifactSource: filepath.Clean(abs), workspace: workspace,
		laneCounts:     map[tracequery.PairingEndpointFamily]int64{},
		familyPoisoned: map[tracequery.PairingEndpointFamily]bool{},
	}
	if err := stage.open(ctx); err != nil {
		return nil, errors.Join(err, stage.cleanup())
	}
	return stage, nil
}

func (stage *traceDBRawPairingStage) open(ctx context.Context) error {
	if err := applyTraceDBSQLiteHardHeapLimit(); err != nil {
		return err
	}
	stage.maxPages = stage.options.MaxTempBytes / traceDBRawPairingSQLitePageBytes
	if stage.maxPages < traceDBRawPairingSQLiteMinimumPages {
		return stage.failBudget(traceDBRawPairingBudgetSQLitePageCap)
	}
	stage.dbPath = filepath.Join(stage.workspace, "raw-pairing-stage.sqlite")
	db, err := sql.Open("sqlite", stage.dbPath)
	if err != nil {
		return fmt.Errorf("open private raw pairing stage: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	stage.db = db
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire private raw pairing stage connection: %w", err)
	}
	stage.conn = conn
	if err := conn.PingContext(ctx); err != nil {
		return fmt.Errorf("create private raw pairing stage: %w", err)
	}
	if err := os.Chmod(stage.dbPath, 0o600); err != nil {
		return fmt.Errorf("secure private raw pairing stage: %w", err)
	}
	pragmas := []string{
		"PRAGMA page_size=4096", "PRAGMA journal_mode=OFF", "PRAGMA synchronous=OFF",
		"PRAGMA temp_store=MEMORY", "PRAGMA locking_mode=EXCLUSIVE", "PRAGMA mmap_size=0",
		fmt.Sprintf("PRAGMA cache_size=-%d", stage.options.SQLiteCache),
		fmt.Sprintf("PRAGMA max_page_count=%d", stage.maxPages),
	}
	for _, pragma := range pragmas {
		if _, err := conn.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("configure private raw pairing stage: %w", err)
		}
	}
	var pageSize, maxPages int64
	var journalMode string
	var mmapSize, tempStore int64
	if err := conn.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err != nil || pageSize != traceDBRawPairingSQLitePageBytes {
		return errors.Join(&traceDBOutputInvariantError{Reason: "raw_pairing_stage_page_size_not_applied"}, err)
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA max_page_count").Scan(&maxPages); err != nil || maxPages <= 0 || maxPages > stage.maxPages {
		return errors.Join(&traceDBOutputInvariantError{Reason: "raw_pairing_stage_page_cap_not_applied"}, err)
	}
	stage.maxPages = maxPages
	stage.baseMaxPages = maxPages
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil || strings.ToLower(journalMode) != "off" {
		return errors.Join(&traceDBOutputInvariantError{Reason: "raw_pairing_stage_journal_not_disabled"}, err)
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA mmap_size").Scan(&mmapSize); err != nil || mmapSize != 0 {
		return errors.Join(&traceDBOutputInvariantError{Reason: "raw_pairing_stage_mmap_not_disabled"}, err)
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA temp_store").Scan(&tempStore); err != nil || tempStore != 2 {
		return errors.Join(&traceDBOutputInvariantError{Reason: "raw_pairing_stage_temp_store_not_memory"}, err)
	}
	for _, ddl := range traceDBRawPairingStageSchema {
		if _, err := conn.ExecContext(ctx, ddl); err != nil {
			return stage.handleWriteError(ctx, fmt.Errorf("initialize private raw pairing stage: %w", err))
		}
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return stage.handleWriteError(ctx, fmt.Errorf("begin private raw pairing stage: %w", err))
	}
	stage.tx = tx
	preparations := []struct {
		dst **sql.Stmt
		sql string
	}{
		{&stage.insertRecord, traceDBRawPairingInsertRecordSQL},
		{&stage.upsertStable, `INSERT INTO stable_identity(stable_id,occurrences) VALUES(?,1)
			ON CONFLICT(stable_id) DO UPDATE SET occurrences=occurrences+1`},
		{&stage.insertDuplicate, `INSERT OR IGNORE INTO duplicate_stable(stable_id) VALUES(?)`},
		{&stage.insertLaneSeen, `INSERT OR IGNORE INTO lane_seen(family,lane_key) VALUES(?,?)`},
		{&stage.insertPoisonLane, `INSERT OR IGNORE INTO poison_lane(family,lane_key) VALUES(?,?)`},
		{&stage.insertPoisonFamily, `INSERT OR IGNORE INTO poison_family(family) VALUES(?)`},
	}
	for _, item := range preparations {
		statement, err := tx.PrepareContext(ctx, item.sql)
		if err != nil {
			return stage.handleWriteError(ctx, err)
		}
		*item.dst = statement
	}
	return nil
}

func (stage *traceDBRawPairingStage) add(ctx context.Context, observation traceDBRawPairingObservation) error {
	if err := stage.requireOpen(ctx); err != nil {
		return err
	}
	if err := validateTraceDBRawPairingObservation(observation); err != nil {
		return err
	}
	if stage.budgetReason != "" {
		return &traceDBRawPairingStageBudgetError{Reason: stage.budgetReason}
	}
	if stage.records >= stage.options.MaxRecords {
		return stage.failBudget(traceDBRawPairingBudgetRecordCap)
	}
	stage.records++
	if observation.StableKnown {
		if err := stage.ensurePageHeadroom(ctx, traceDBRawPairingSQLiteWriteReservePages); err != nil {
			return err
		}
		result, err := stage.upsertStable.ExecContext(ctx, observation.StableID)
		if err != nil {
			return stage.handleWriteError(ctx, err)
		}
		if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
			return errors.Join(&traceDBOutputInvariantError{Reason: "raw_pairing_stage_stable_upsert_count"}, rowsErr)
		}
		var occurrences int64
		if err := stage.tx.QueryRowContext(ctx, `SELECT occurrences FROM stable_identity WHERE stable_id=?`, observation.StableID).Scan(&occurrences); err != nil {
			return stage.handleWriteError(ctx, err)
		}
		if occurrences > 1 {
			if _, err := stage.insertDuplicate.ExecContext(ctx, observation.StableID); err != nil {
				return stage.handleWriteError(ctx, err)
			}
		}
	}

	endpoint := observation.Verdict.Recognized
	if endpoint && observation.Verdict.Family == "" {
		return &traceDBOutputInvariantError{Reason: "raw_pairing_stage_missing_endpoint_family"}
	}
	if endpoint && observation.LaneKey != "" {
		if err := stage.admitLaneKey(ctx, observation.Verdict.Family, observation.LaneKey); err != nil {
			return err
		}
	}
	if endpoint && !observation.EndpointAdmitted {
		if observation.HeaderOwnerKnown && observation.Verdict.KeyKnown && observation.LaneKey != "" {
			if err := stage.poisonLane(ctx, observation.Verdict.Family, observation.LaneKey); err != nil {
				return err
			}
		} else if err := stage.poisonFamily(ctx, observation.Verdict.Family); err != nil {
			return err
		}
	}
	store := observation.Publishable || endpoint
	if !store {
		return nil
	}
	if observation.Publishable && (!observation.StableKnown || !observation.TimestampKnown || observation.Class == "" || observation.Line == "") {
		return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_publishable_record"}
	}
	payloadBytes := int64(len(observation.Line) + len(observation.Class) + len(observation.LaneKey))
	if payloadBytes < 0 || payloadBytes > math.MaxInt64-2048 {
		return stage.failBudget(traceDBRawPairingBudgetSQLitePageCap)
	}
	requiredPages := (payloadBytes+2048+traceDBRawPairingSQLitePageBytes-1)/traceDBRawPairingSQLitePageBytes + traceDBRawPairingSQLiteWriteReservePages
	if err := stage.ensurePageHeadroom(ctx, requiredPages); err != nil {
		return err
	}
	stage.stored++
	stableOrder := observation.StableID
	if !observation.StableKnown {
		stableOrder = stage.records
	}
	laneKey := any(nil)
	if observation.LaneKey != "" {
		laneKey = []byte(observation.LaneKey)
	}
	result, err := stage.insertRecord.ExecContext(ctx,
		stage.records, observation.StableID, boolToSQLiteInt(observation.StableKnown), stableOrder,
		observation.Timestamp, observation.Class, observation.Line, boolToSQLiteInt(observation.Publishable),
		boolToSQLiteInt(endpoint), string(observation.Verdict.Family), string(observation.Verdict.Phase), laneKey,
		boolToSQLiteInt(observation.HeaderOwnerKnown), observation.CanonicalITID, boolToSQLiteInt(observation.CanonicalITIDKnown),
	)
	if err != nil {
		return stage.handleWriteError(ctx, err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return errors.Join(&traceDBOutputInvariantError{Reason: "raw_pairing_stage_record_insert_count"}, rowsErr)
	}
	return nil
}

func validateTraceDBRawPairingObservation(observation traceDBRawPairingObservation) error {
	if observation.StableKnown && observation.StableID < 0 ||
		observation.TimestampKnown && observation.Timestamp < 0 ||
		observation.CanonicalITIDKnown && (observation.CanonicalITID < 0 || observation.CanonicalITID > maxTraceDBInternalID) ||
		len(observation.LaneKey) > maxTraceDBSystraceLineBytes {
		return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_observation_scalar"}
	}
	endpoint := observation.Verdict.Recognized
	if endpoint {
		if !traceDBRawPairingFamilyValid(observation.Verdict.Family) ||
			(observation.Verdict.Phase != tracequery.PairingEndpointStart && observation.Verdict.Phase != tracequery.PairingEndpointDone) ||
			observation.Verdict.KeyKnown != (observation.LaneKey != "") {
			return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_observation_endpoint"}
		}
		if observation.EndpointAdmitted && (!observation.Publishable || !observation.HeaderOwnerKnown ||
			!observation.TimestampKnown || !observation.Verdict.KeyKnown || !observation.Verdict.PayloadAdmitted ||
			!observation.Verdict.EmitterKnown || !observation.Verdict.EmitterAdmitted) {
			return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_observation_admission"}
		}
	} else if observation.EndpointAdmitted || observation.LaneKey != "" || observation.Verdict.Family != "" || observation.Verdict.Phase != "" {
		return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_observation_non_endpoint"}
	}
	if observation.Publishable && (!observation.StableKnown || !observation.TimestampKnown || observation.Class == "" || observation.Line == "") {
		return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_publishable_record"}
	}
	return nil
}

func traceDBRawPairingFamilyValid(family tracequery.PairingEndpointFamily) bool {
	switch family {
	case tracequery.PairingEndpointBinder, tracequery.PairingEndpointWorkqueue, tracequery.PairingEndpointDMAFence,
		tracequery.PairingEndpointBlock, tracequery.PairingEndpointStorage:
		return true
	default:
		return false
	}
}

func (stage *traceDBRawPairingStage) admitLaneKey(ctx context.Context, family tracequery.PairingEndpointFamily, laneKey string) error {
	if stage.familyPoisoned[family] {
		return nil
	}
	result, err := stage.insertLaneSeen.ExecContext(ctx, string(family), []byte(laneKey))
	if err != nil {
		return stage.handleWriteError(ctx, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return nil
	}
	if rows != 1 {
		return &traceDBOutputInvariantError{Reason: "raw_pairing_stage_lane_seen_insert_count"}
	}
	stage.laneCounts[family]++
	if stage.laneCounts[family] > stage.options.MaxLaneKeys {
		if err := stage.poisonFamily(ctx, family); err != nil {
			return err
		}
		return &traceDBRawPairingStageBudgetError{Reason: traceDBRawPairingBudgetLaneKeyCap}
	}
	return nil
}

func (stage *traceDBRawPairingStage) poisonLane(ctx context.Context, family tracequery.PairingEndpointFamily, laneKey string) error {
	if stage.familyPoisoned[family] || family == "" || laneKey == "" {
		return nil
	}
	if err := stage.ensurePageHeadroom(ctx, traceDBRawPairingSQLiteWriteReservePages); err != nil {
		return err
	}
	if _, err := stage.insertPoisonLane.ExecContext(ctx, string(family), []byte(laneKey)); err != nil {
		return stage.handleWriteError(ctx, err)
	}
	return nil
}

func (stage *traceDBRawPairingStage) poisonFamily(ctx context.Context, family tracequery.PairingEndpointFamily) error {
	if family == "" || stage.familyPoisoned[family] {
		return nil
	}
	if err := stage.ensurePageHeadroom(ctx, traceDBRawPairingSQLiteWriteReservePages); err != nil {
		return err
	}
	if _, err := stage.insertPoisonFamily.ExecContext(ctx, string(family)); err != nil {
		return stage.handleWriteError(ctx, err)
	}
	stage.familyPoisoned[family] = true
	return nil
}

const traceDBRawPairingAuditSQL = `SELECT family,phase,lane_key,ts_ns,canonical_itid,canonical_known
	FROM record INDEXED BY raw_record_lane_audit_idx
	WHERE endpoint=1 AND publishable=1
	ORDER BY family,lane_key,stable_order,ordinal`

var (
	traceDBRawPairingJournalHeader = []byte("CDRXRPJ1")
	traceDBRawPairingJournalFooter = []byte("RPJEND01")
)

const traceDBRawPairingJournalFooterBytes int64 = 8 + 8 + 4

type traceDBRawPairingPoisonJournal struct {
	stage     *traceDBRawPairingStage
	path      string
	file      *os.File
	writer    *bufio.Writer
	count     int64
	dataBytes int64
	closed    bool
}

func (stage *traceDBRawPairingStage) auditAdmittedLanes(ctx context.Context) error {
	if err := stage.verifyAuditPlan(ctx); err != nil {
		return err
	}
	journal, err := newTraceDBRawPairingPoisonJournal(ctx, stage)
	if err != nil {
		return err
	}
	removeJournal := true
	defer func() {
		if removeJournal {
			_ = journal.remove()
		}
	}()
	rows, err := stage.tx.QueryContext(ctx, traceDBRawPairingAuditSQL)
	if err != nil {
		return fmt.Errorf("open raw pairing lane audit: %w", err)
	}
	var currentFamily tracequery.PairingEndpointFamily
	var currentLane []byte
	haveLane, laneBad, haveTimestamp := false, false, false
	var lastTimestamp, openCount, openITID int64
	openKnown := false
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		var familyText, phaseText string
		var lane []byte
		var timestamp, canonicalITID, canonicalKnownRaw int64
		if err := rows.Scan(&familyText, &phaseText, &lane, &timestamp, &canonicalITID, &canonicalKnownRaw); err != nil {
			_ = rows.Close()
			return err
		}
		family := tracequery.PairingEndpointFamily(familyText)
		phase := tracequery.PairingEndpointPhase(phaseText)
		canonicalKnown, boolOK := traceDBSQLiteExactBool(canonicalKnownRaw)
		if !traceDBRawPairingFamilyValid(family) || (phase != tracequery.PairingEndpointStart && phase != tracequery.PairingEndpointDone) ||
			len(lane) == 0 || len(lane) > maxTraceDBSystraceLineBytes || timestamp < 0 || !boolOK ||
			canonicalKnown && (canonicalITID < 0 || canonicalITID > maxTraceDBInternalID) {
			_ = rows.Close()
			return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_audit_record"}
		}
		if !haveLane || family != currentFamily || !bytes.Equal(lane, currentLane) {
			currentFamily, currentLane = family, append(currentLane[:0], lane...)
			haveLane, laneBad, haveTimestamp = true, false, false
			lastTimestamp, openCount, openITID, openKnown = 0, 0, 0, false
		}
		if laneBad || stage.familyPoisoned[family] {
			continue
		}
		bad := haveTimestamp && timestamp < lastTimestamp
		if !haveTimestamp || timestamp > lastTimestamp {
			lastTimestamp = timestamp
		}
		haveTimestamp = true
		if !bad && (family == tracequery.PairingEndpointWorkqueue || family == tracequery.PairingEndpointDMAFence || family == tracequery.PairingEndpointStorage) {
			if !canonicalKnown {
				bad = true
			} else {
				switch phase {
				case tracequery.PairingEndpointStart:
					if openCount == math.MaxInt64 || openCount > 0 && (!openKnown || openITID != canonicalITID) {
						bad = true
					} else {
						if openCount == 0 {
							openITID, openKnown = canonicalITID, true
						}
						openCount++
					}
				case tracequery.PairingEndpointDone:
					if openCount > 0 {
						if !openKnown || openITID != canonicalITID {
							bad = true
						} else {
							openCount--
							if openCount == 0 {
								openITID, openKnown = 0, false
							}
						}
					}
				}
			}
		}
		if bad {
			if err := journal.add(ctx, family, lane); err != nil {
				_ = rows.Close()
				return err
			}
			laneBad = true
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := journal.seal(); err != nil {
		return err
	}
	if err := journal.replay(ctx); err != nil {
		return err
	}
	if err := journal.remove(); err != nil {
		return err
	}
	removeJournal = false
	if err := stage.setActiveMaxPages(ctx, stage.baseMaxPages); err != nil {
		return err
	}
	return nil
}

func newTraceDBRawPairingPoisonJournal(ctx context.Context, stage *traceDBRawPairingStage) (*traceDBRawPairingPoisonJournal, error) {
	if err := stage.ensureAuditFileHeadroom(ctx, 0, int64(len(traceDBRawPairingJournalHeader))); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(stage.workspace, "raw-pairing-audit-*.bin")
	if err != nil {
		return nil, fmt.Errorf("create raw pairing audit journal: %w", err)
	}
	journal := &traceDBRawPairingPoisonJournal{stage: stage, path: file.Name(), file: file, writer: bufio.NewWriterSize(file, 64<<10)}
	if err := os.Chmod(journal.path, 0o600); err != nil {
		return nil, errors.Join(fmt.Errorf("secure raw pairing audit journal: %w", err), journal.remove())
	}
	if _, err := journal.writer.Write(traceDBRawPairingJournalHeader); err != nil {
		return nil, errors.Join(err, journal.remove())
	}
	journal.dataBytes = int64(len(traceDBRawPairingJournalHeader))
	return journal, nil
}

func (journal *traceDBRawPairingPoisonJournal) add(ctx context.Context, family tracequery.PairingEndpointFamily, lane []byte) error {
	if journal == nil || journal.closed || journal.writer == nil || !traceDBRawPairingFamilyValid(family) || len(lane) == 0 || len(lane) > maxTraceDBSystraceLineBytes {
		return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_audit_journal_record"}
	}
	if journal.count >= journal.stage.options.MaxRecords {
		return journal.stage.failBudget(traceDBRawPairingBudgetRecordCap)
	}
	recordBytes := int64(1 + 4 + len(lane))
	if err := journal.stage.ensureAuditFileHeadroom(ctx, journal.dataBytes, recordBytes); err != nil {
		return err
	}
	code, ok := traceDBRawPairingFamilyCode(family)
	if !ok {
		return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_audit_journal_family"}
	}
	if err := journal.writer.WriteByte(code); err != nil {
		return err
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(lane)))
	if _, err := journal.writer.Write(length[:]); err != nil {
		return err
	}
	if _, err := journal.writer.Write(lane); err != nil {
		return err
	}
	journal.count++
	journal.dataBytes += recordBytes
	return nil
}

func (journal *traceDBRawPairingPoisonJournal) seal() error {
	if journal == nil || journal.closed || journal.file == nil || journal.writer == nil {
		return &traceDBOutputInvariantError{Reason: "raw_pairing_audit_journal_not_open"}
	}
	if err := journal.writer.Flush(); err != nil {
		return err
	}
	if _, err := journal.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	checksum := crc32.NewIEEE()
	if copied, err := io.CopyN(checksum, journal.file, journal.dataBytes); err != nil || copied != journal.dataBytes {
		return errors.Join(&traceDBOutputInvariantError{Reason: "raw_pairing_audit_journal_checksum_source"}, err)
	}
	if _, err := journal.file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	footer := make([]byte, traceDBRawPairingJournalFooterBytes)
	copy(footer, traceDBRawPairingJournalFooter)
	binary.BigEndian.PutUint64(footer[8:16], uint64(journal.count))
	binary.BigEndian.PutUint32(footer[16:20], checksum.Sum32())
	if _, err := journal.file.Write(footer); err != nil {
		return err
	}
	if err := journal.file.Sync(); err != nil {
		return err
	}
	if err := journal.file.Close(); err != nil {
		return err
	}
	journal.file, journal.writer, journal.closed = nil, nil, true
	return nil
}

func (journal *traceDBRawPairingPoisonJournal) replay(ctx context.Context) error {
	if journal == nil || !journal.closed || journal.path == "" {
		return &traceDBOutputInvariantError{Reason: "raw_pairing_audit_journal_not_sealed"}
	}
	file, err := os.Open(journal.path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() != journal.dataBytes+traceDBRawPairingJournalFooterBytes || journal.count < 0 || journal.count > journal.stage.options.MaxRecords {
		return &traceDBOutputInvariantError{Reason: "raw_pairing_audit_journal_size"}
	}
	footer := make([]byte, traceDBRawPairingJournalFooterBytes)
	if _, err := file.ReadAt(footer, journal.dataBytes); err != nil {
		return err
	}
	if !bytes.Equal(footer[:8], traceDBRawPairingJournalFooter) || binary.BigEndian.Uint64(footer[8:16]) != uint64(journal.count) {
		return &traceDBOutputInvariantError{Reason: "raw_pairing_audit_journal_footer"}
	}
	checksum := crc32.NewIEEE()
	if copied, err := io.CopyN(checksum, io.NewSectionReader(file, 0, journal.dataBytes), journal.dataBytes); err != nil || copied != journal.dataBytes {
		return errors.Join(&traceDBOutputInvariantError{Reason: "raw_pairing_audit_journal_checksum_read"}, err)
	}
	if checksum.Sum32() != binary.BigEndian.Uint32(footer[16:20]) {
		return &traceDBOutputInvariantError{Reason: "raw_pairing_audit_journal_checksum_mismatch"}
	}
	if err := journal.stage.constrainAuditReplayPages(ctx, info.Size()); err != nil {
		return err
	}
	reader := bufio.NewReaderSize(io.NewSectionReader(file, 0, journal.dataBytes), 64<<10)
	header := make([]byte, len(traceDBRawPairingJournalHeader))
	if _, err := io.ReadFull(reader, header); err != nil || !bytes.Equal(header, traceDBRawPairingJournalHeader) {
		return errors.Join(&traceDBOutputInvariantError{Reason: "raw_pairing_audit_journal_header"}, err)
	}
	for index := int64(0); index < journal.count; index++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		code, err := reader.ReadByte()
		if err != nil {
			return err
		}
		var length [4]byte
		if _, err := io.ReadFull(reader, length[:]); err != nil {
			return err
		}
		size := binary.BigEndian.Uint32(length[:])
		if size == 0 || uint64(size) > uint64(maxTraceDBSystraceLineBytes) {
			return &traceDBOutputInvariantError{Reason: "raw_pairing_audit_journal_key_size"}
		}
		lane := make([]byte, int(size))
		if _, err := io.ReadFull(reader, lane); err != nil {
			return err
		}
		family, ok := traceDBRawPairingFamilyFromCode(code)
		if !ok {
			return &traceDBOutputInvariantError{Reason: "raw_pairing_audit_journal_family"}
		}
		if err := journal.stage.poisonLaneUnderAuditCap(ctx, family, string(lane), info.Size()); err != nil {
			return err
		}
	}
	if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
		return errors.Join(&traceDBOutputInvariantError{Reason: "raw_pairing_audit_journal_trailing_data"}, err)
	}
	return nil
}

// constrainAuditReplayPages converts the combined SQLite+journal byte limit
// into SQLite's own hard page limit while the sealed journal is present. This
// is an enforcement mechanism, not an estimated insert-size gate: SQLite
// cannot allocate a page that would make the two files exceed MaxTempBytes.
func (stage *traceDBRawPairingStage) constrainAuditReplayPages(ctx context.Context, journalBytes int64) error {
	if journalBytes <= 0 || journalBytes > stage.options.MaxTempBytes || stage.tx == nil || stage.baseMaxPages <= 0 {
		return stage.failBudget(traceDBRawPairingBudgetTempByteCap)
	}
	availablePages := (stage.options.MaxTempBytes - journalBytes) / traceDBRawPairingSQLitePageBytes
	var pages int64
	if err := stage.tx.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return err
	}
	if availablePages < pages || availablePages <= 0 {
		return stage.failBudget(traceDBRawPairingBudgetTempByteCap)
	}
	if availablePages > stage.baseMaxPages {
		availablePages = stage.baseMaxPages
	}
	if err := stage.setActiveMaxPages(ctx, availablePages); err != nil {
		return err
	}
	return stage.sampleAuditCombinedBytes(ctx, journalBytes)
}

func (stage *traceDBRawPairingStage) poisonLaneUnderAuditCap(ctx context.Context, family tracequery.PairingEndpointFamily, laneKey string, journalBytes int64) error {
	if stage.familyPoisoned[family] || family == "" || laneKey == "" {
		return nil
	}
	if _, err := stage.insertPoisonLane.ExecContext(ctx, string(family), []byte(laneKey)); err != nil {
		if traceDBSQLitePrimaryErrorCode(err) == 13 {
			return stage.failBudget(traceDBRawPairingBudgetTempByteCap)
		}
		return stage.handleWriteError(ctx, err)
	}
	return stage.sampleAuditCombinedBytes(ctx, journalBytes)
}

func (stage *traceDBRawPairingStage) sampleAuditCombinedBytes(ctx context.Context, journalBytes int64) error {
	if journalBytes < 0 || stage.tx == nil {
		return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_audit_combined_sample"}
	}
	var pages int64
	if err := stage.tx.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return err
	}
	if pages < 0 || pages > math.MaxInt64/traceDBRawPairingSQLitePageBytes {
		return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_audit_page_count"}
	}
	dbBytes := pages * traceDBRawPairingSQLitePageBytes
	if journalBytes > math.MaxInt64-dbBytes {
		return stage.failBudget(traceDBRawPairingBudgetTempByteCap)
	}
	combined := dbBytes + journalBytes
	if combined > stage.options.MaxTempBytes {
		return stage.failBudget(traceDBRawPairingBudgetTempByteCap)
	}
	if combined > stage.peakTempBytes {
		stage.peakTempBytes = combined
	}
	return nil
}

func (journal *traceDBRawPairingPoisonJournal) remove() error {
	if journal == nil {
		return nil
	}
	var errs []error
	if journal.writer != nil {
		errs = append(errs, journal.writer.Flush())
		journal.writer = nil
	}
	if journal.file != nil {
		errs = append(errs, journal.file.Close())
		journal.file = nil
	}
	if journal.path != "" {
		if err := os.Remove(journal.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
		journal.path = ""
	}
	journal.closed = true
	return errors.Join(errs...)
}

func traceDBRawPairingFamilyCode(family tracequery.PairingEndpointFamily) (byte, bool) {
	switch family {
	case tracequery.PairingEndpointBinder:
		return 1, true
	case tracequery.PairingEndpointWorkqueue:
		return 2, true
	case tracequery.PairingEndpointDMAFence:
		return 3, true
	case tracequery.PairingEndpointBlock:
		return 4, true
	case tracequery.PairingEndpointStorage:
		return 5, true
	default:
		return 0, false
	}
}

func traceDBRawPairingFamilyFromCode(code byte) (tracequery.PairingEndpointFamily, bool) {
	switch code {
	case 1:
		return tracequery.PairingEndpointBinder, true
	case 2:
		return tracequery.PairingEndpointWorkqueue, true
	case 3:
		return tracequery.PairingEndpointDMAFence, true
	case 4:
		return tracequery.PairingEndpointBlock, true
	case 5:
		return tracequery.PairingEndpointStorage, true
	default:
		return "", false
	}
}

func (stage *traceDBRawPairingStage) ensureAuditFileHeadroom(ctx context.Context, currentFileBytes, delta int64) error {
	if delta < 0 || currentFileBytes < 0 || stage.tx == nil {
		return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_audit_reservation"}
	}
	var pages int64
	if err := stage.tx.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return err
	}
	dbBytes := pages * traceDBRawPairingSQLitePageBytes
	needed := currentFileBytes + delta + traceDBRawPairingJournalFooterBytes
	if needed < 0 || needed > stage.options.MaxTempBytes || dbBytes > stage.options.MaxTempBytes-needed {
		return stage.failBudget(traceDBRawPairingBudgetTempByteCap)
	}
	if peak := dbBytes + needed; peak > stage.peakTempBytes {
		stage.peakTempBytes = peak
	}
	return nil
}

func (stage *traceDBRawPairingStage) verifyAuditPlan(ctx context.Context) error {
	rows, err := stage.tx.QueryContext(ctx, "EXPLAIN QUERY PLAN "+traceDBRawPairingAuditSQL)
	if err != nil {
		return fmt.Errorf("explain raw pairing lane audit: %w", err)
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
	if !strings.Contains(plan, "RAW_RECORD_LANE_AUDIT_IDX") || strings.Contains(plan, "TEMP B-TREE") {
		return &traceDBOutputInvariantError{Reason: "raw_pairing_stage_unbounded_audit_query_plan"}
	}
	return nil
}

func (stage *traceDBRawPairingStage) seal(ctx context.Context) (traceDBRawPairingStageReport, error) {
	report := traceDBRawPairingStageReport{PhysicalRows: stage.records, StoredRows: stage.stored, PeakTempBytes: stage.peakTempBytes, BudgetReason: stage.budgetReason}
	if err := stage.requireOpen(ctx); err != nil {
		return report, err
	}
	if stage.budgetReason != "" {
		return report, &traceDBRawPairingStageBudgetError{Reason: stage.budgetReason}
	}
	if err := stage.auditAdmittedLanes(ctx); err != nil {
		return report, err
	}
	// Duplicate stable identities remove every duplicate row. Governed rows
	// additionally quarantine the complete lane so their neighbors cannot pair
	// across the removed physical endpoint.
	duplicatePoison := []string{
		`INSERT OR IGNORE INTO poison_lane(family,lane_key)
		 SELECT r.family,r.lane_key FROM record r JOIN duplicate_stable d ON d.stable_id=r.stable_id
		 WHERE r.endpoint=1 AND r.owner_known=1 AND r.lane_key IS NOT NULL`,
		`INSERT OR IGNORE INTO poison_family(family)
		 SELECT DISTINCT r.family FROM record r JOIN duplicate_stable d ON d.stable_id=r.stable_id
		 WHERE r.endpoint=1 AND (r.owner_known=0 OR r.lane_key IS NULL)`,
	}
	for _, statement := range duplicatePoison {
		if _, err := stage.tx.ExecContext(ctx, statement); err != nil {
			return report, stage.handleWriteError(ctx, err)
		}
	}
	if err := stage.closeWriteStatements(); err != nil {
		return report, err
	}
	tx := stage.tx
	stage.tx = nil
	if tx == nil {
		return report, &traceDBOutputInvariantError{Reason: "raw_pairing_stage_missing_write_transaction"}
	}
	if err := tx.Commit(); err != nil {
		return report, stage.handleWriteError(ctx, fmt.Errorf("commit private raw pairing stage: %w", err))
	}
	if err := stage.sampleTempBytes(); err != nil {
		return report, err
	}
	if err := stage.verifyPublishPlan(ctx); err != nil {
		return report, err
	}
	stage.state = traceDBRawPairingStageSealed
	if err := stage.conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(s.occurrences),0) FROM stable_identity s JOIN duplicate_stable d ON d.stable_id=s.stable_id`).Scan(&report.DuplicatePhysicalRows); err != nil {
		return report, err
	}
	if err := stage.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM poison_lane`).Scan(&report.PoisonedLanes); err != nil {
		return report, err
	}
	if err := stage.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM poison_family`).Scan(&report.PoisonedFamilies); err != nil {
		return report, err
	}
	report.PeakTempBytes = stage.peakTempBytes
	return report, nil
}

func (stage *traceDBRawPairingStage) publish(ctx context.Context, sink *traceDBRowSink, emittedByClass map[string]int) (traceDBRawPairingStageReport, error) {
	report := traceDBRawPairingStageReport{PhysicalRows: stage.records, StoredRows: stage.stored, PeakTempBytes: stage.peakTempBytes, BudgetReason: stage.budgetReason}
	if stage == nil || stage.closed || stage.state != traceDBRawPairingStageSealed || sink == nil || emittedByClass == nil || ctx == nil {
		return report, &traceDBOutputInvariantError{Reason: "raw_pairing_stage_not_sealed_for_publish"}
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	countSQL := `SELECT COUNT(*) ` + traceDBRawPairingPublishFrom + ` WHERE ` + traceDBRawPairingPublishWhere
	var publishCount int64
	if err := stage.conn.QueryRowContext(ctx, countSQL).Scan(&publishCount); err != nil {
		return report, err
	}
	if publishCount < 0 || sink.stats.RowsAccepted < 0 || publishCount > int64(math.MaxInt-sink.stats.RowsAccepted) {
		report.BudgetReason = traceDBRawPairingBudgetSequenceCap
		return report, &traceDBRawPairingStageBudgetError{Reason: traceDBRawPairingBudgetSequenceCap}
	}
	var totalPublishable int64
	if err := stage.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM record WHERE publishable=1`).Scan(&totalPublishable); err != nil {
		return report, err
	}
	report.SuppressedRows = totalPublishable - publishCount
	query := `SELECT r.class,r.ts_ns,r.line ` + traceDBRawPairingPublishFrom + ` WHERE ` + traceDBRawPairingPublishWhere + ` ORDER BY r.ts_ns,r.stable_order,r.ordinal`
	rows, err := stage.conn.QueryContext(ctx, query)
	if err != nil {
		return report, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		var class, line string
		var ts int64
		if err := rows.Scan(&class, &ts, &line); err != nil {
			return report, err
		}
		if err := traceDBPublishFrozenRawRecord(sink, ts, line); err != nil {
			return report, err
		}
		emittedByClass[class]++
		report.PublishedRows++
	}
	if err := rows.Err(); err != nil {
		return report, err
	}
	if report.PublishedRows != publishCount {
		return report, &traceDBOutputInvariantError{Reason: "raw_pairing_stage_publish_count_mismatch"}
	}
	stage.state = traceDBRawPairingStagePublished
	return report, nil
}

// traceDBPublishFrozenRawRecord is the sole raw-stage sink publisher. The
// validated line was rendered in pass 1; pass 2 supplies only the final global
// sequence after all lane/family barriers are frozen.
func traceDBPublishFrozenRawRecord(sink *traceDBRowSink, ts int64, line string) error {
	if sink == nil || ts < 0 || !traceDBSinglePhysicalLine(line, false) {
		return &traceDBOutputInvariantError{Reason: "invalid_frozen_raw_record"}
	}
	return sink.add(renderedRow{tsNS: uint64(ts), seq: sink.stats.RowsAccepted, line: line})
}

func (stage *traceDBRawPairingStage) verifyPublishPlan(ctx context.Context) error {
	query := `EXPLAIN QUERY PLAN SELECT r.class,r.ts_ns,r.line ` + traceDBRawPairingPublishFrom + ` WHERE ` + traceDBRawPairingPublishWhere + ` ORDER BY r.ts_ns,r.stable_order,r.ordinal`
	rows, err := stage.conn.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("explain raw pairing publish iterator: %w", err)
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
	if !strings.Contains(plan, "RAW_RECORD_PUBLISH_IDX") || strings.Contains(plan, "TEMP B-TREE") {
		return &traceDBOutputInvariantError{Reason: "raw_pairing_stage_unbounded_publish_query_plan"}
	}
	return nil
}

func (stage *traceDBRawPairingStage) requireOpen(ctx context.Context) error {
	if stage == nil || stage.closed || stage.state != traceDBRawPairingStageOpen || stage.tx == nil {
		return &traceDBOutputInvariantError{Reason: "raw_pairing_stage_not_open"}
	}
	if ctx == nil {
		return &traceDBOutputInvariantError{Reason: "missing_raw_pairing_stage_context"}
	}
	return ctx.Err()
}

func (stage *traceDBRawPairingStage) ensurePageHeadroom(ctx context.Context, requiredPages int64) error {
	if requiredPages <= 0 || stage.maxPages <= 0 || stage.tx == nil {
		return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_stage_page_reservation"}
	}
	var pages int64
	if err := stage.tx.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return fmt.Errorf("read raw pairing stage page count: %w", err)
	}
	bytes := pages * traceDBRawPairingSQLitePageBytes
	if bytes > stage.peakTempBytes {
		stage.peakTempBytes = bytes
	}
	if requiredPages > stage.maxPages || pages > stage.maxPages-requiredPages {
		return stage.failBudget(traceDBRawPairingBudgetSQLitePageCap)
	}
	return nil
}

func (stage *traceDBRawPairingStage) setActiveMaxPages(ctx context.Context, maxPages int64) error {
	if stage.tx == nil || maxPages <= 0 || stage.baseMaxPages <= 0 || maxPages > stage.baseMaxPages {
		return &traceDBOutputInvariantError{Reason: "invalid_raw_pairing_stage_page_limit"}
	}
	var currentPages int64
	if err := stage.tx.QueryRowContext(ctx, "PRAGMA page_count").Scan(&currentPages); err != nil {
		return err
	}
	if maxPages < currentPages {
		return stage.failBudget(traceDBRawPairingBudgetTempByteCap)
	}
	var applied int64
	pragma := fmt.Sprintf("PRAGMA max_page_count=%d", maxPages)
	if err := stage.tx.QueryRowContext(ctx, pragma).Scan(&applied); err != nil {
		return err
	}
	if applied != maxPages {
		return &traceDBOutputInvariantError{Reason: "raw_pairing_stage_page_limit_not_applied"}
	}
	stage.maxPages = applied
	return nil
}

func (stage *traceDBRawPairingStage) handleWriteError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return errors.Join(ctxErr, err)
	}
	if traceDBSQLitePrimaryErrorCode(err) == 13 && stage.maxPages > 0 {
		var pages int64
		if stage.tx != nil {
			if pageErr := stage.tx.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); pageErr == nil && pages >= stage.maxPages {
				return stage.failBudget(traceDBRawPairingBudgetSQLitePageCap)
			}
		}
	}
	stage.state = traceDBRawPairingStageFailed
	return fmt.Errorf("private raw pairing stage write: %w", err)
}

func (stage *traceDBRawPairingStage) failBudget(reason string) error {
	if stage.budgetReason == "" {
		stage.budgetReason = reason
	}
	return &traceDBRawPairingStageBudgetError{Reason: stage.budgetReason}
}

func (stage *traceDBRawPairingStage) sampleTempBytes() error {
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
			return stage.failBudget(traceDBRawPairingBudgetTempByteCap)
		}
		total += info.Size()
	}
	if total > stage.peakTempBytes {
		stage.peakTempBytes = total
	}
	if total > stage.options.MaxTempBytes {
		return stage.failBudget(traceDBRawPairingBudgetTempByteCap)
	}
	return nil
}

func (stage *traceDBRawPairingStage) closeWriteStatements() error {
	var errs []error
	for _, statement := range []*sql.Stmt{
		stage.insertRecord, stage.upsertStable, stage.insertDuplicate, stage.insertLaneSeen,
		stage.insertPoisonLane, stage.insertPoisonFamily,
	} {
		if statement != nil {
			errs = append(errs, statement.Close())
		}
	}
	stage.insertRecord, stage.upsertStable, stage.insertDuplicate, stage.insertLaneSeen = nil, nil, nil, nil
	stage.insertPoisonLane, stage.insertPoisonFamily = nil, nil
	return errors.Join(errs...)
}

func (stage *traceDBRawPairingStage) cleanup() error {
	if stage == nil || stage.closed {
		if stage == nil {
			return nil
		}
		return stage.cleanupErr
	}
	stage.closed = true
	var errs []error
	errs = append(errs, stage.closeWriteStatements())
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
	for _, path := range []string{stage.dbPath, stage.dbPath + "-journal", stage.dbPath + "-wal", stage.dbPath + "-shm"} {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if stage.workspace != "" {
		errs = append(errs, os.RemoveAll(stage.workspace))
	}
	stage.cleanupErr = errors.Join(errs...)
	return stage.cleanupErr
}
