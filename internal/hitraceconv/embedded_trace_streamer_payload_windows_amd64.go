//go:build !slim_streamer && windows && amd64

package hitraceconv

// windows-amd64 embedded trace_streamer payload (HED-59 ruling
// 2026-07-05; default-embed ruling 2026-07-13). Per-platform
// distribution: every windows/amd64 build embeds the windows binary
// alone unless it explicitly opts out with slim_streamer, and never
// pays the linux payload bytes. Unrelated build tags do not disable the
// default payload.

import (
	"embed"
	"io/fs"
)

//go:embed embedded_trace_streamer/windows-amd64
var embeddedTraceStreamerEmbedFS embed.FS

func init() {
	embeddedTraceStreamerTagEnabled = true
	embeddedTraceStreamerAssetsFS = func() fs.FS {
		sub, err := fs.Sub(embeddedTraceStreamerEmbedFS, embeddedTraceStreamerDir+"/windows-amd64")
		if err != nil {
			return nil
		}
		return sub
	}
}
