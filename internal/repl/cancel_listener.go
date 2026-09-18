package repl

import "github.com/hanchaoqun/codrax/internal/types"

// cancelListenerQueueCap preserves the bounded runtime follow-up admission
// policy. The shared script input owner also limits cumulative queued bytes.
const cancelListenerQueueCap = 32

func truncateForWarn(s string, max int) string {
	return types.TruncateBytesEllipsis(s, max)
}
