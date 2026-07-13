package hitraceconv

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReleaseTraceStreamerDBStagingIsPrivate pins the directory boundary for
// both transient and retained exports. trace_streamer may create DB companions
// in this directory, so group/world access is not acceptable even briefly.
func TestReleaseTraceStreamerDBStagingIsPrivate(t *testing.T) {
	tests := []struct {
		name     string
		retained bool
	}{
		{name: "transient"},
		{name: "retained", retained: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			opts := Options{}
			if tc.retained {
				opts.TraceDBOutputPath = filepath.Join(dir, "retained.trace.db")
			}
			target, err := prepareTraceStreamerDBTarget(opts, filepath.Join(dir, "input.sys"), filepath.Join(dir, "out.systrace"), false)
			if err != nil {
				t.Fatal(err)
			}
			stagingDir := filepath.Dir(target.StagingPath)
			info, err := os.Lstat(stagingDir)
			if err != nil {
				t.Fatal(err)
			}
			if !info.IsDir() || info.Mode().Perm() != 0o700 {
				t.Fatalf("trace DB staging is not private: path=%s mode=%s", stagingDir, info.Mode())
			}
			if err := cleanupTraceStreamerDBTarget(target.Cleanup); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(stagingDir); !os.IsNotExist(err) {
				t.Fatalf("private staging directory survived cleanup: %s err=%v", stagingDir, err)
			}
		})
	}
}
