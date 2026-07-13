//go:build !slim_streamer && linux && amd64

package hitraceconv

// linux-amd64 embedded trace_streamer payload (HED-59 ruling
// 2026-07-05; default-embed ruling 2026-07-13). Per-platform
// distribution: every linux/amd64 build embeds the linux binary alone
// unless it explicitly opts out with slim_streamer, and never pays the
// windows payload bytes. Unrelated build tags do not disable the
// default payload.

import (
	"embed"
	"io/fs"
)

//go:embed embedded_trace_streamer/linux-amd64
var embeddedTraceStreamerEmbedFS embed.FS

func init() {
	embeddedTraceStreamerTagEnabled = true
	embeddedTraceStreamerAssetsFS = func() fs.FS {
		sub, err := fs.Sub(embeddedTraceStreamerEmbedFS, embeddedTraceStreamerDir+"/linux-amd64")
		if err != nil {
			return nil
		}
		return sub
	}
}
