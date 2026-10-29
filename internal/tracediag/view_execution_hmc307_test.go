package tracediag

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// HMC308 adds the real packet adapter; the original read-before-admission
// protection now rejects incompatible coordinates rather than the whole view.
func TestHMC307ScriptRejectsPacketTraceCoordinatesBeforeAccess(t *testing.T) {
	for _, version := range []int{1, 2} {
		for _, view := range []string{"packet_inventory", "packet_flows"} {
			t.Run(fmt.Sprintf("v%d/%s", version, view), func(t *testing.T) {
				_, err := ParseScript([]byte(fmt.Sprintf("version: %d\ndefaults: {pid: 41, window: '1..2'}\nsteps:\n  - {label: packets, view: %s}\n", version, view)))
				if err == nil || !strings.Contains(err.Error(), "tracediag") || !strings.Contains(err.Error(), "execution") {
					t.Fatalf("packet Trace coordinates must fail execution admission: %v", err)
				}
				path := filepath.Join(t.TempDir(), "script.yaml")
				if err := os.WriteFile(path, []byte(fmt.Sprintf("version: %d\ndefaults: {window: '1..2'}\nsteps:\n  - {label: packets, view: %s}\n", version, view)), 0600); err != nil {
					t.Fatal(err)
				}
				var report bytes.Buffer
				_, err = Run(context.Background(), Options{ScriptPath: path, TracePath: path + ".absent"}, &report)
				if err == nil || !strings.Contains(err.Error(), "packet execution") || report.Len() != 0 {
					t.Fatalf("must reject before source read/report success: %v report=%q", err, report.String())
				}
			})
		}
	}
}

func TestHMC307ScriptKeepsImplementedEntries(t *testing.T) {
	for _, version := range []int{1, 2} {
		for _, view := range []string{"event_search", "window_sweep", "format_census", "frame_root_cause_bundle"} {
			t.Run(fmt.Sprintf("v%d/%s", version, view), func(t *testing.T) {
				_, err := ParseScript([]byte(fmt.Sprintf("version: %d\ndefaults: {pid: 41, window: '1..2'}\nsteps:\n  - {label: trace, view: %s}\n", version, view)))
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
