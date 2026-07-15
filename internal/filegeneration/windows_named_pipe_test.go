package filegeneration

import "testing"

func TestIsWindowsNamedPipePathClosedSpellings(t *testing.T) {
	for _, path := range []string{
		`\\.\pipe\capture`,
		`\\?\PIPE\capture`,
		`\\server\pipe\capture`,
		`//server/PIPE/capture`,
		`\\?\UNC\server\pipe\capture`,
		`\\?\GLOBALROOT\Device\NamedPipe\capture`,
		`\\.\GLOBALROOT\Device\NamedPipe\capture`,
	} {
		if !IsWindowsNamedPipePath(path) {
			t.Errorf("named-pipe spelling was admitted: %q", path)
		}
	}
	for _, path := range []string{
		`C:\trace\pipe\capture.systrace`,
		`\\server\traces\capture.systrace`,
		`\\?\UNC\server\traces\capture.systrace`,
		`pipe\capture`,
		`\\server\pipeline\capture`,
	} {
		if IsWindowsNamedPipePath(path) {
			t.Errorf("regular path was rejected as a named pipe: %q", path)
		}
	}
}
