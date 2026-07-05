//go:build embed_streamer && linux && amd64

package hitraceconv

// linux-amd64 embedded trace_streamer payload (HED-59 ruling
// 2026-07-05). Per-platform distribution: this file compiles only into
// linux/amd64 builds carrying the embed_streamer tag, so a linux
// release embeds the linux binary alone and never pays the windows
// payload bytes (and vice versa). Slim builds (no tag) compile none of
// the payload stubs and contain zero embedded bytes.

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
