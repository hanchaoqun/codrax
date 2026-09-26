package hitraceconv

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func traceDBHiSysName(raw any, name, reason string) tracewire.HiSysEventName {
	n := tracewire.HiSysEventName{Status: reason}
	if id, ok := traceDBStrictSQLiteInt(raw); ok {
		n.Reference = &id
	}
	if reason == "" {
		n.Status = "resolved"
		n.Name = &name
	}
	return n
}

func traceDBHiSysContents(raw any) (tracewire.HiSysEventContents, error) {
	var c tracewire.HiSysEventContents
	var value string
	switch v := raw.(type) {
	case nil:
		c.StorageClass = "null"
		return c, nil
	case string:
		if !utf8.ValidString(v) {
			return c, &traceDBOutputInvariantError{Reason: "invalid_body"}
		}
		c.StorageClass, value = "text", v
	case []byte:
		if len(v) > maxTraceDBSystraceLineBytes {
			return c, &traceDBOutputInvariantError{Reason: "line_too_long"}
		}
		c.StorageClass, c.BytesBase64 = "blob", base64.StdEncoding.EncodeToString(v)
		return c, nil
	case int64:
		c.StorageClass, value = "integer", strconv.FormatInt(v, 10)
	case float64:
		c.StorageClass, value = "real", strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return c, &traceDBOutputInvariantError{Reason: "invalid_body", Cause: fmt.Errorf("unsupported HiSys contents storage")}
	}
	if len(value) > maxTraceDBSystraceLineBytes {
		return c, &traceDBOutputInvariantError{Reason: "line_too_long"}
	}
	c.Text = &value
	return c, nil
}

func addTraceDBHiSysObservationRow(sink *traceDBRowSink, row tracewire.HiSysEvent) error {
	var tid int64
	if row.SourceTID != nil {
		tid = *row.SourceTID
	}
	// Retain the original scalar validity gates without inserting arbitrary
	// payload bytes into an ftrace envelope. Encoding is not header repair.
	if _, err := prepareTraceDBRenderedRow(row.TimestampNS, sink.stats.RowsAccepted, "<hisysevent>", tid, tid, 0, "print: observation"); err != nil {
		return err
	}
	for _, name := range []*string{row.Domain.Name, row.Event.Name} {
		if name != nil && !utf8.ValidString(*name) {
			return &traceDBOutputInvariantError{Reason: "invalid_body"}
		}
	}
	line, err := tracewire.FormatHiSysEventObservation(row)
	if err != nil {
		return &traceDBOutputInvariantError{Reason: "line_too_long", Cause: err}
	}
	return sink.add(renderedRow{tsNS: uint64(row.TimestampNS), seq: sink.stats.RowsAccepted, line: line})
}
