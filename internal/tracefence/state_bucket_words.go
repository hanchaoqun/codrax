package tracefence

// StateNonIODStateWord names the residual D accounting bucket, retaining the
// historical API name. Both a known-zero IO marker and an unknown marker can
// enter this bucket; its membership does not prove a non-IO mechanism. It is
// not the physical D-state total (this bucket + scheduler-marked IO). Keep
// per-interval marker knowledge and StateLaneDState's folded word separate.
func StateNonIODStateWord(zh bool) string {
	if zh {
		return "D 状态（未标记为 IO 等待）"
	}
	return "D-state without an IO-wait mark"
}
