package tracequery

import (
	"math"
	"strconv"
	"strings"
)

// binderTransactionTypedFields is the capture-local, occurrence-aware wire
// verdict for binder_transaction and binder_transaction_received. It is
// cached by lineScan so Event construction and the retained endpoint adapter
// consume one grammar. Values are fixed-width before they ever enter Event's
// compatibility int fields.
type binderTransactionTypedFields struct {
	TransactionID    int
	DestProc         int
	DestThread       int
	Reply            int
	Flags            string
	Code             string
	FlagsValue       uint32
	CodeValue        uint32
	TransactionKnown bool
	DestProcKnown    bool
	DestThreadKnown  bool
	ReplyKnown       bool
	FlagsKnown       bool
	CodeKnown        bool
	LexValid         bool
	DebugAlias       bool
}

// parseBinderTransactionTypedFields is the sole text argset decoder for the
// two Binder endpoint rows. The shared pairing lexer is quote-aware and keeps
// physical occurrences; no last-write-wins map is consulted for hard fields.
func parseBinderTransactionTypedFields(rawType, fields string) (map[string]string, binderTransactionTypedFields) {
	typed := binderTransactionTypedFields{}
	tokens, lexOK := tokenizePairingKV(fields)
	typed.LexValid = lexOK
	if !lexOK {
		return map[string]string{}, typed
	}

	transaction, transactionOK, debugAlias := strictBinderTransactionIdentityTokens(tokens, true)
	if transactionOK {
		value, _ := strconv.ParseUint(transaction, 10, 31)
		typed.TransactionID = int(value)
		typed.TransactionKnown = true
		typed.DebugAlias = debugAlias
	}
	if strings.TrimSpace(rawType) == string(EventBinderReceived) {
		out := map[string]string{}
		if typed.TransactionKnown {
			out["transaction"] = transaction
		}
		return out, typed
	}

	if raw, present, unique := strictBinderFieldOccurrence(tokens, "dest_proc"); present && unique {
		if value, ok := parseCanonicalBinderPID(raw, false); ok {
			typed.DestProc, typed.DestProcKnown = value, true
		}
	}
	if raw, present, unique := strictBinderFieldOccurrence(tokens, "dest_thread"); present && unique {
		if value, ok := parseCanonicalBinderPID(raw, true); ok {
			typed.DestThread, typed.DestThreadKnown = value, true
		}
	}
	if raw, present, unique := strictBinderFieldOccurrence(tokens, "reply"); present && unique {
		if raw == "0" || raw == "1" {
			typed.Reply, typed.ReplyKnown = int(raw[0]-'0'), true
		}
	}
	if raw, present, unique := strictBinderFieldOccurrence(tokens, "flags"); present && unique {
		if value, canonical, ok := parseCanonicalBinderU32Hex(raw); ok {
			typed.FlagsValue, typed.Flags, typed.FlagsKnown = value, canonical, true
		}
	}
	if raw, present, unique := strictBinderFieldOccurrence(tokens, "code"); present && unique {
		if value, canonical, ok := parseCanonicalBinderU32Hex(raw); ok {
			typed.CodeValue, typed.Code, typed.CodeKnown = value, canonical, true
		}
	}

	out := map[string]string{}
	if typed.TransactionKnown {
		out["transaction"] = transaction
	}
	if typed.DestProcKnown {
		out["dest_proc"] = strconv.Itoa(typed.DestProc)
	}
	if typed.DestThreadKnown {
		out["dest_thread"] = strconv.Itoa(typed.DestThread)
	}
	if typed.ReplyKnown {
		out["reply"] = strconv.Itoa(typed.Reply)
	}
	if typed.FlagsKnown {
		out["flags"] = typed.Flags
	}
	if typed.CodeKnown {
		out["code"] = typed.Code
	}
	return out, typed
}

func strictBinderTransactionIdentityTokens(tokens []pairingKVToken, lexOK bool) (canonical string, ok, debugAlias bool) {
	if !lexOK {
		return "", false, false
	}
	found := false
	for _, token := range tokens {
		switch token.key {
		case "transaction", "debug_id", "transaction_id":
		default:
			continue
		}
		if found {
			return "", false, false
		}
		value, valid := canonicalBinderTransactionIdentity(token.rawValue)
		if !valid {
			return "", false, false
		}
		canonical, found = value, true
		debugAlias = token.key == "debug_id"
	}
	return canonical, found, debugAlias
}

func canonicalBinderTransactionIdentity(raw string) (string, bool) {
	raw, ok := strictBinderUnquotedScalar(raw)
	// Keep the established endpoint compatibility for zero-padded decimal
	// transaction IDs, but canonicalize before key construction. The producer
	// emits plain decimal; quotes, signs, fractions and int32 overflow remain
	// invalid.
	if !ok || !isAllDigits(raw) {
		return "", false
	}
	value, err := strconv.ParseUint(raw, 10, 31)
	if err != nil || value == 0 || value > math.MaxInt32 {
		return "", false
	}
	return strconv.FormatUint(value, 10), true
}

func strictBinderFieldOccurrence(tokens []pairingKVToken, key string) (raw string, present, unique bool) {
	unique = true
	for _, token := range tokens {
		if token.key != key {
			continue
		}
		if present {
			return "", true, false
		}
		raw, present = token.rawValue, true
	}
	if !present {
		return "", false, false
	}
	raw, unique = strictBinderUnquotedScalar(raw)
	return raw, true, unique && raw != ""
}

func strictBinderUnquotedScalar(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] == '\'' || raw[0] == '"' || raw[len(raw)-1] == '\'' || raw[len(raw)-1] == '"' {
		return "", false
	}
	if len(raw) > 256 || strings.ContainsAny(raw, "'\"=, \t\r\n") {
		return "", false
	}
	return raw, true
}

func parseCanonicalBinderPID(raw string, zeroAllowed bool) (int, bool) {
	if !canonicalUnsignedDecimal(raw) {
		return 0, false
	}
	value, err := strconv.ParseUint(raw, 10, 31)
	if err != nil || (!zeroAllowed && value == 0) || value > math.MaxInt32 {
		return 0, false
	}
	return int(value), true
}

func canonicalUnsignedDecimal(raw string) bool {
	if raw == "" || !isAllDigits(raw) || (len(raw) > 1 && raw[0] == '0') {
		return false
	}
	return true
}

func parseCanonicalBinderU32Hex(raw string) (uint32, string, bool) {
	if !strings.HasPrefix(raw, "0x") || len(raw) <= 2 {
		return 0, "", false
	}
	digits := raw[2:]
	if len(digits) > 1 && digits[0] == '0' {
		return 0, "", false
	}
	for i := 0; i < len(digits); i++ {
		if !((digits[i] >= '0' && digits[i] <= '9') || (digits[i] >= 'a' && digits[i] <= 'f')) {
			return 0, "", false
		}
	}
	value, err := strconv.ParseUint(digits, 16, 32)
	if err != nil || value > math.MaxUint32 {
		return 0, "", false
	}
	return uint32(value), "0x" + strconv.FormatUint(value, 16), true
}

func (typed binderTransactionTypedFields) binderFields(intern *stringInterner) *BinderFields {
	flags, code := typed.Flags, typed.Code
	if intern != nil {
		flags, code = intern.intern(flags), intern.intern(code)
	}
	debugID := 0
	if typed.DebugAlias && typed.TransactionKnown {
		debugID = typed.TransactionID
	}
	return &BinderFields{
		TransactionID:    typed.TransactionID,
		DestProc:         typed.DestProc,
		DestThread:       typed.DestThread,
		Reply:            typed.Reply,
		Flags:            flags,
		Code:             code,
		DebugID:          debugID,
		argsetParsed:     true,
		argsetLexValid:   typed.LexValid,
		transactionKnown: typed.TransactionKnown,
		destProcKnown:    typed.DestProcKnown,
		destThreadKnown:  typed.DestThreadKnown,
		replyKnown:       typed.ReplyKnown,
		flagsKnown:       typed.FlagsKnown,
		codeKnown:        typed.CodeKnown,
		flagsValue:       typed.FlagsValue,
		codeValue:        typed.CodeValue,
	}
}

func binderFieldsForEdge(send Event) *BinderFields {
	if send.BinderFields != nil && send.BinderFields.argsetParsed {
		return send.BinderFields
	}
	if strings.TrimSpace(send.FieldText) != "" {
		_, typed := parseBinderTransactionTypedFields(string(EventBinderTransaction), send.FieldText)
		return typed.binderFields(nil)
	}
	if send.BinderFields != nil {
		return send.BinderFields
	}
	return &BinderFields{}
}

func (bf *BinderFields) binderTransactionKnown() bool {
	if bf == nil {
		return false
	}
	if bf.argsetParsed {
		return bf.transactionKnown
	}
	return bf.TransactionID > 0 && bf.TransactionID <= math.MaxInt32
}

func (bf *BinderFields) binderDestinationKnown() bool {
	if bf == nil {
		return false
	}
	if bf.argsetParsed {
		return bf.destProcKnown && bf.destThreadKnown
	}
	return bf.DestProc > 0 && bf.DestProc <= math.MaxInt32 && bf.DestThread >= 0 && bf.DestThread <= math.MaxInt32
}

func (bf *BinderFields) binderReplyKnown() bool {
	if bf == nil {
		return false
	}
	if bf.argsetParsed {
		return bf.replyKnown
	}
	return bf.Reply == 0 || bf.Reply == 1
}

func (bf *BinderFields) binderFlags() (uint32, string, bool) {
	if bf == nil {
		return 0, "", false
	}
	if bf.argsetParsed {
		return bf.flagsValue, bf.Flags, bf.flagsKnown
	}
	value, canonical, ok := parseCanonicalBinderU32Hex(strings.TrimSpace(bf.Flags))
	return value, canonical, ok
}

func (bf *BinderFields) binderCode() (string, bool) {
	if bf == nil {
		return "", false
	}
	if bf.argsetParsed {
		return bf.Code, bf.codeKnown
	}
	_, canonical, ok := parseCanonicalBinderU32Hex(strings.TrimSpace(bf.Code))
	return canonical, ok
}
