//go:build codrax_embed_trace_streamer

package hitraceconv

import (
	"embed"
	"io/fs"
)

//go:embed embedded_trace_streamer/*
var embeddedTraceStreamerEmbedFS embed.FS

func init() {
	embeddedTraceStreamerAssetsFS = func() fs.FS {
		sub, err := fs.Sub(embeddedTraceStreamerEmbedFS, embeddedTraceStreamerDir)
		if err != nil {
			return nil
		}
		return sub
	}
}
