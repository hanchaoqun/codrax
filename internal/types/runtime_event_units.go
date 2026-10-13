package types

import (
	"strconv"
	"strings"
)

// Add only equivalent units for already admitted typed fields. This does not
// infer a duration, change a time domain, or parse an untyped payload value.
func appendRuntimeEventMillisecondFields(fields []TraceEventSemanticField, descriptors []RuntimeEventFieldDescriptor) ([]TraceEventSemanticField, []RuntimeEventFieldDescriptor) {
	for _, d := range descriptors {
		if d.Type != "int64" || d.Unit != "ns" || !strings.HasSuffix(d.Key, "_ns") {
			continue
		}
		f, found := runtimeEventField(RuntimeEventRow{Fields: fields}, d.Key)
		if !found {
			continue
		}
		d.Key, d.Type, d.Unit = strings.TrimSuffix(d.Key, "_ns")+"_ms", "decimal", "ms"
		f.Key, f.Type, f.Unit = d.Key, d.Type, d.Unit
		if f.Status == "known" && f.Value != nil {
			value, ok := runtimeEventNanosecondsAsMilliseconds(*f.Value)
			if !ok {
				// The inventory validator already rejects malformed int64 values.
				// Keep this pure projection safe if called before that boundary.
				continue
			}
			f.Value = &value
		}
		if f.Omitted != nil {
			omission := *f.Omitted
			f.Omitted = &omission
		}
		fields, descriptors = append(fields, f), append(descriptors, d)
	}
	return fields, descriptors
}

func runtimeEventNanosecondsAsMilliseconds(value string) (string, bool) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return "", false
	}
	// Decimal placement avoids both float precision loss and abs(MinInt64).
	digits := strings.TrimPrefix(strconv.FormatInt(n, 10), "-")
	if len(digits) <= 6 {
		digits = strings.Repeat("0", 7-len(digits)) + digits
	}
	whole, fraction := digits[:len(digits)-6], strings.TrimRight(digits[len(digits)-6:], "0")
	if fraction != "" {
		whole += "." + fraction
	}
	if n < 0 {
		whole = "-" + whole
	}
	return whole, true
}
