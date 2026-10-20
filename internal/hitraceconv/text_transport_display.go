package hitraceconv

// TextTransportDisplay selects complete-text publication metadata for human
// displays. It grants no query/evidence authority and does not count events.
// Consumers must not infer this state from a suffix, output path or zero count.
func TextTransportDisplay(result Result) (path string, size int64, member string, ok bool) {
	if transport := result.TextTransport; transport != nil && transport.Profile == GzipTextTransportProfile && result.ArchiveTextTransport == nil {
		return transport.DecodedPath, transport.DecodedBytes, "", true
	}
	if transport := result.ArchiveTextTransport; transport != nil && transport.Profile == ArchiveTextTransportProfile && result.TextTransport == nil {
		return transport.DecodedPath, transport.DecodedBytes, transport.Archive.Member, true
	}
	return "", 0, "", false
}
