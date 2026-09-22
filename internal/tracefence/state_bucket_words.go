package tracefence

// StateNonIODStateWord names the exclusive non-IO D accounting bucket. It
// does not name the physical D-state total (D + scheduler-marked IO), and
// the bucket alone does not establish that an underlying IO mechanism is
// absent. Keep this separate from StateLaneDState's published folded word.
func StateNonIODStateWord(zh bool) string {
	if zh {
		return "非 IO D-state"
	}
	return "non-IO D-state"
}
