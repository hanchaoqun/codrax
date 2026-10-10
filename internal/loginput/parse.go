package loginput

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	wallPrefix   = regexp.MustCompile(`^\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?`)
	hilogLine    = regexp.MustCompile(`^(\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{1,9})\s+(\d+)\s+(\d+)\s+([VDIWEF])\s+([^:]+):\s?(.*)$`)
	kmsgWallLine = regexp.MustCompile(`^(\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{1,9})\s+<(\d+)>\s*\[\s*(\d+(?:\.\d+)?)\]\s*(.*)$`)
	kmsgBootLine = regexp.MustCompile(`^(?:<(\d+)>)?\[\s*(\d+(?:\.\d+)?)\]\s*(.*)$`)
	kmsgContext  = regexp.MustCompile(`^-;\[(\d+)\]\s+pid=(\d+)\s+tid=(\d+)\s+comm=(\S+)\s*(.*)$`)
)

func parseRecord(text string) (Record, bool) {
	if m := kmsgWallLine.FindStringSubmatch(text); m != nil {
		record := parseKmsg(m[2], m[3], m[4])
		record.WallTimestamp = m[1]
		if !validWallTimestamp(m[1]) {
			record.Status, record.ParseError = "malformed", "invalid_wall_timestamp"
		}
		return record, true
	}
	if m := hilogLine.FindStringSubmatch(text); m != nil {
		record := Record{Kind: KindHilog, Status: "parsed", WallTimestamp: m[1], Level: m[4], Tag: strings.TrimSpace(m[5]), Message: m[6], ClockDomain: "wall_year_and_timezone_unknown"}
		var err error
		record.PID, err = decimalID(m[2])
		if err == nil {
			record.TID, err = decimalID(m[3])
		}
		if err != nil {
			record.Status, record.ParseError = "malformed", "invalid_process_or_thread_id"
		}
		if !validWallTimestamp(m[1]) {
			record.Status, record.ParseError = "malformed", "invalid_wall_timestamp"
		}
		return record, true
	}
	if m := kmsgBootLine.FindStringSubmatch(text); m != nil {
		return parseKmsg(m[1], m[2], m[3]), true
	}
	if wallPrefix.MatchString(text) {
		return Record{Kind: KindText, Status: "malformed", WallTimestamp: wallPrefix.FindString(text), ParseError: "unrecognized_timestamped_record"}, true
	}
	return Record{Kind: KindText, Status: "unknown", Message: text}, false
}

func parseKmsg(level, boot, rest string) Record {
	record := Record{Kind: KindKmsg, Status: "parsed", Level: level, BootTimestamp: boot, ClockDomain: "source_boot_unmapped", Message: rest}
	var err error
	record.BootTimestampNS, err = decimalSecondsNS(boot)
	if err != nil {
		record.Status, record.ParseError = "malformed", "invalid_boot_timestamp"
	}
	if m := kmsgContext.FindStringSubmatch(rest); m != nil {
		record.CPU, err = decimalID(m[1])
		if err == nil {
			record.PID, err = decimalID(m[2])
		}
		if err == nil {
			record.TID, err = decimalID(m[3])
		}
		if err != nil {
			record.Status, record.ParseError = "malformed", "invalid_execution_context_id"
		}
		record.Comm, record.Message = m[4], m[5]
	} else if strings.HasPrefix(rest, "-;[") {
		record.Status, record.ParseError = "malformed", "invalid_execution_context"
	}
	return record
}

func decimalID(value string) (*int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// Decimal concatenation never passes through float64, even beyond 2^53 ns.
func decimalSecondsNS(value string) (string, error) {
	parts := strings.Split(value, ".")
	if len(parts) > 2 || len(parts[0]) == 0 {
		return "", fmt.Errorf("invalid decimal seconds")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) == 0 || len(fraction) > 9 {
			return "", fmt.Errorf("sub-nanosecond precision is unsupported")
		}
	}
	for _, part := range parts {
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return "", fmt.Errorf("invalid decimal seconds")
			}
		}
	}
	ns := strings.TrimLeft(parts[0]+fraction+strings.Repeat("0", 9-len(fraction)), "0")
	if ns == "" {
		ns = "0"
	}
	return ns, nil
}

// Validate the partial civil timestamp without supplying a year or timezone.
// February 29 remains possible because the source does not state a year.
func validWallTimestamp(value string) bool {
	if len(value) < 16 {
		return false
	}
	parts := []string{value[0:2], value[3:5], value[6:8], value[9:11], value[12:14]}
	values := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return false
		}
		values[i] = n
	}
	month, day := values[0], values[1]
	if month < 1 || month > 12 || day < 1 || values[2] > 23 || values[3] > 59 || values[4] > 59 {
		return false
	}
	days := []int{31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	return day <= days[month-1]
}
