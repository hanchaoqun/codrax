//go:build embed_streamer && windows && amd64

package hitraceconv

// windows-amd64 embedded trace_streamer payload (HED-59 ruling
// 2026-07-05). Per-platform distribution: this file compiles only into
// windows/amd64 builds carrying the embed_streamer tag, so a windows
// release embeds the windows binary alone and never pays the linux
// payload bytes (and vice versa). Slim builds (no tag) compile none of
// the payload stubs and contain zero embedded bytes.

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
