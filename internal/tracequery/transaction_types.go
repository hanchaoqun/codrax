package tracequery

const ViewTransactionHandoffs = "transaction_handoffs"

const TransactionHandoffsTeaching = "transaction_handoffs correlates exact Harmony application MarshRSTransactionData transactionFlag:[tid,seq] submissions with RSMainThread::ProcessCommandUni key lists across the complete frozen published source. It reports observed-unique, ambiguous, missing or identity-unverified protocol matches, with each endpoint's own query-window membership. Use time_start/time_end and optional pid/thread/target_scope; no event/name/line filters. Multiple keys in one consumption preserve branches. Identity uses actual emitter TID, recorded TGID and absence of observed lifecycle conflicts, not original database ITID/IPID. Unpublished/rejected source rows remain outside this observation universe. A protocol match proves neither a complete frame, thread waiting, GPU execution nor a response root cause; missing or reused keys are not matched by proximity."

const TransactionHandoffsLimit = 48
const TransactionEndpointExamplesLimit = 4

type TransactionEndpoint struct {
	SourcePath string  `json:"source_path"`
	SourceLine int     `json:"source_line"`
	Line       int     `json:"line"`
	Ts         float64 `json:"ts"`
	TID        int     `json:"tid"`
	TGID       int     `json:"tgid"`
	Thread     string  `json:"thread"`
	Name       string  `json:"name"`
	InWindow   bool    `json:"in_window"`
}

type TransactionHandoff struct {
	TID                 int                   `json:"key_tid"`
	Sequence            string                `json:"sequence"`
	Status              string                `json:"status"`
	Reason              string                `json:"reason,omitempty"`
	SubmissionCount     int                   `json:"submission_count"`
	ConsumptionCount    int                   `json:"consumption_count"`
	WindowSubmissions   int                   `json:"window_submissions"`
	WindowConsumptions  int                   `json:"window_consumptions"`
	Submissions         []TransactionEndpoint `json:"submissions,omitempty"`
	Consumptions        []TransactionEndpoint `json:"consumptions,omitempty"`
	OmittedSubmissions  int                   `json:"omitted_submissions"`
	OmittedConsumptions int                   `json:"omitted_consumptions"`
}

type TransactionHandoffsResult struct {
	Status                  string                    `json:"status"`
	Reason                  string                    `json:"reason,omitempty"`
	SourcePath              string                    `json:"source_path"`
	Window                  RenderingCandidatesWindow `json:"window"`
	TargetPID               int                       `json:"target_pid,omitempty"`
	TargetThread            string                    `json:"target_thread,omitempty"`
	TargetScope             string                    `json:"target_scope,omitempty"`
	TotalKeys               int                       `json:"total_keys"`
	OmittedKeys             int                       `json:"omitted_keys"`
	WindowSubmissionEvents  int                       `json:"window_submission_events"`
	WindowConsumptionEvents int                       `json:"window_consumption_events"`
	MalformedProtocolEvents int                       `json:"malformed_protocol_events"`
	Handoffs                []TransactionHandoff      `json:"handoffs,omitempty"`
	Caveats                 []string                  `json:"caveats,omitempty"`
}
