package tracequery

import "strings"

// blockedReasonSemanticCaller separates a usable symbol from raw-address
// provenance. Pure kernel addresses are ASLR/build-specific identities, not
// stable reason classes; admitting them into Event.Reason fragments the
// blocked-reason Top-N into high-cardinality count=1 buckets. The complete raw
// payload remains byte-for-byte available through Event.FieldText.
func blockedReasonSemanticCaller(kv map[string]string) string {
	caller := strings.TrimSpace(kv["caller"])
	quality := strings.ToLower(strings.TrimSpace(kv["caller_quality"]))
	if quality == "opaque" || caller == "" || strings.EqualFold(caller, "unknown") || isPureHexAddressToken(caller) {
		return "unknown"
	}
	return caller
}

func isPureHexAddressToken(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) <= 2 || value[0] != '0' || (value[1] != 'x' && value[1] != 'X') {
		return false
	}
	for _, c := range value[2:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
