package tracequery

import (
	"sort"
	"strings"
)

type frameVideoTrack struct {
	path         string
	surfaceToken string
	queued       []uint64
}

type frameVideoProducer struct {
	process int
	thread  int
	surface string
	decoder bool
}

type frameCatalogVideo struct {
	tracks  map[frameVideoProducer]*frameVideoTrack
	pending map[int]*frameVideoTrack
}

func (v *frameCatalogVideo) observe(tid, tgid int, timestamp uint64, name string, decoder bool) {
	guestSubmission := strings.HasPrefix(name, "ProducerSurfaceDelegator::QueueBuffer ")
	if name == "queueBuffer" || guestSubmission {
		if track := v.pending[tid]; track != nil && (track.path == "") == guestSubmission {
			track.queued = append(track.queued, timestamp)
			delete(v.pending, tid)
		}
		return
	}
	surface, ok := strings.CutPrefix(name, "dequeueBuffer - ")
	if !ok {
		return
	}
	token, _, surfaceView := strings.Cut(surface, " SurfaceView[")
	var path string
	switch {
	case decoder && strings.Contains(surface, "SurfaceView["):
		path = "surface_view"
	case decoder && strings.HasPrefix(surface, "SurfaceTexture-"):
		path = "surface_texture"
	case surfaceView && token != "":
		// A guest layer producer need not have a decoder thread name.
		// Its explicit window submission supplies host-layer evidence only.
	default:
		delete(v.pending, tid)
		return
	}
	key := frameVideoProducer{tgid, tid, surface, decoder}
	track := v.tracks[key]
	if track == nil {
		track = &frameVideoTrack{path: path}
		if surfaceView {
			track.surfaceToken = token
		}
		v.tracks[key] = track
	}
	v.pending[tid] = track
}

func (v *frameCatalogVideo) attach(frames []FrameCatalogEntry) map[int]string {
	surfaceTokens := make(map[int]string)
	for i := range frames {
		f := &frames[i]
		var decoderTrack, layerTrack *frameVideoTrack
		var decoders, layers int
		for key, track := range v.tracks {
			if key.process != f.ProcessID {
				continue
			}
			at := sort.Search(len(track.queued), func(i int) bool { return track.queued[i] > f.StartNS })
			if at < 3 || f.StartNS-track.queued[at-1] > 125_000_000 || f.StartNS-track.queued[at-3] > 250_000_000 {
				continue
			}
			if track.path != "" {
				decoderTrack = track
				decoders++
			} else {
				layerTrack = track
				layers++
			}
		}
		switch {
		case decoders == 1:
			f.VideoOutputPath = decoderTrack.path
			if decoderTrack.surfaceToken != "" {
				surfaceTokens[f.StartLine] = decoderTrack.surfaceToken
			}
		case decoders == 0 && layers == 1:
			surfaceTokens[f.StartLine] = layerTrack.surfaceToken
		}
	}
	return surfaceTokens
}
