//go:build darwin

package hitraceconv

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type hmc276LifecycleResult struct {
	hmc275CaptureResult
	SHMCloseChanged bool `json:"shm_close_generation_changed"`
}

// Synchronization stays outside SQLite callbacks and never changes source data.
// Admission and pin are different boundaries: BEGIN alone is not a snapshot.
func TestHMC276WALLifecycleHelper(t *testing.T) {
	if os.Getenv("CODRAX_HMC276_WAL_HELPER") != "1" {
		t.Skip("isolated subprocess only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	scanner := bufio.NewScanner(os.Stdin)
	var observed *hmc275WALVFS
	steps := 0
	cancelAfterPage := false
	boundary := func(stage string) func(*hmc275WALVFS) error {
		return func(v *hmc275WALVFS) error {
			observed = v
			fmt.Println("HMC276_" + stage)
			if !scanner.Scan() {
				return fmt.Errorf("controller closed at %s", stage)
			}
			switch scanner.Text() {
			case "resume":
			case "cancel_after_page":
				cancelAfterPage = true
			default:
				return fmt.Errorf("unexpected controller command at %s", stage)
			}
			return nil
		}
	}
	path, err := hmc275CaptureAtAdmission(ctx, os.Getenv("CODRAX_HMC275_SOURCE"), os.Getenv("CODRAX_HMC275_OUTPUT"), boundary("ADMITTED"), boundary("PINNED"), func(n int) {
		steps = n
		if cancelAfterPage {
			cancel()
		}
	})
	r := hmc276LifecycleResult{hmc275CaptureResult: hmc275CaptureResult{Path: path, Steps: steps}}
	if err != nil {
		r.Error = err.Error()
	}
	if observed != nil {
		r.Reads, r.SHMLocks, r.DeniedWrites = observed.reads, observed.shmLocks, observed.deniedWrites
		r.SHMCloseChanged = observed.closeSHMChanged
		if r.SHMCloseChanged {
			fmt.Printf("HMC276_CLOSE before=%+v after=%+v\n", observed.closeSHMBefore, observed.closeSHMAfter)
		}
	}
	b, _ := json.Marshal(r)
	fmt.Println("HMC276_RESULT " + string(b))
}

func TestHMC276ActiveWALLifecycle(t *testing.T) {
	for _, mode := range []string{"owner_exit_admitted", "truncate_pinned", "restart_pinned", "checkpoint_cancel_after_page"} {
		t.Run(mode, func(t *testing.T) {
			hmc276RunLifecycle(t, mode, false)
		})
	}
}

// PASS proves this counterexample and late no-publication guard, NOT a safe
// zero-effect snapshot capability. The original red runs are kept separately.
func TestHMC276ActiveWALOwnerExitNoGo(t *testing.T) {
	hmc276RunLifecycle(t, "owner_exit_pinned", true)
}

func hmc276RunLifecycle(t *testing.T, mode string, expectNoGo bool) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.db")
	writer, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	writer.SetMaxOpenConns(1)
	defer writer.Close()
	execSQL := func(q string) {
		t.Helper()
		if _, e := writer.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	execSQL("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; PRAGMA busy_timeout=0; CREATE TABLE first(v INTEGER,payload BLOB); CREATE TABLE second(v INTEGER); INSERT INTO first VALUES(1,zeroblob(262144)); INSERT INTO second VALUES(1); PRAGMA wal_checkpoint(TRUNCATE); BEGIN; UPDATE first SET v=2; UPDATE second SET v=2; COMMIT;")
	observers := make(map[string]*os.File)
	for _, suffix := range []string{"", "-wal", "-shm"} {
		f, e := os.Open(source + suffix)
		if e != nil {
			t.Fatal(e)
		}
		observers[source+suffix] = f
		// Never close a same-inode observation descriptor while this
		// process owns SQLite locks (POSIX fcntl locks are process-wide).
		t.Cleanup(func() { _ = f.Close() })
	}
	before := hmc275ObserveSource(t, observers)
	out := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHMC276WALLifecycleHelper$", "-test.v")
	cmd.Env = append(os.Environ(), "CODRAX_HMC275_WAL_HELPER=1", "CODRAX_HMC276_WAL_HELPER=1", "CODRAX_HMC275_SOURCE="+source, "CODRAX_HMC275_OUTPUT="+out)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		_ = stdin.Close()
		cancel()
		if !waited {
			_ = cmd.Wait()
		}
	}()
	send := func(command string) {
		t.Helper()
		if _, e := ioWriteStringHMC275(stdin, command+"\n"); e != nil {
			t.Fatal(e)
		}
	}
	checkpoint := func(kind string) [3]int {
		t.Helper()
		var r [3]int
		if e := writer.QueryRow("PRAGMA wal_checkpoint("+kind+")").Scan(&r[0], &r[1], &r[2]); e != nil {
			t.Fatal(e)
		}
		t.Logf("checkpoint %s: busy=%d log=%d checkpointed=%d", kind, r[0], r[1], r[2])
		return r
	}
	closeOwner := func() {
		t.Helper()
		if e := writer.Close(); e != nil {
			t.Fatal(e)
		}
		// Any close-triggered checkpoint/delete attempt is the writer's
		// action, not the reader's. Start a fresh no-writer-change window.
		before = hmc275ObserveSource(t, observers)
		t.Log("writer connection closed; held source observed after owner action")
	}
	admitted, pinned, received := false, false, false
	var result hmc276LifecycleResult
	scanner := bufio.NewScanner(stdout)
	var transcript []string
	for scanner.Scan() {
		line := scanner.Text()
		transcript = append(transcript, line)
		switch line {
		case "HMC276_ADMITTED":
			if admitted || pinned {
				t.Fatal("duplicate or out-of-order admission")
			}
			admitted = true
			hmc275AssertUnchanged(t, before)
			if mode == "owner_exit_admitted" {
				closeOwner()
			}
			send("resume")
		case "HMC276_PINNED":
			if !admitted || pinned {
				t.Fatal("duplicate or out-of-order snapshot pin")
			}
			pinned = true
			hmc275AssertUnchanged(t, before)
			if mode == "owner_exit_pinned" {
				closeOwner()
			} else if !strings.HasPrefix(mode, "owner_exit") {
				execSQL("BEGIN; UPDATE first SET v=3; UPDATE second SET v=3; COMMIT;")
				beforeCheckpoint := hmc275ObserveSource(t, observers)
				kind := "TRUNCATE"
				if mode == "restart_pinned" {
					kind = "RESTART"
				}
				r := checkpoint(kind)
				if r[0] != 1 || r[1] <= 0 || r[2] < 0 || r[2] >= r[1] {
					t.Fatalf("checkpoint did not encounter the pinned older transaction: %v", r)
				}
				afterCheckpoint := hmc275ObserveSource(t, observers)
				if !bytes.Equal(beforeCheckpoint[source+"-wal"].bytes, afterCheckpoint[source+"-wal"].bytes) {
					t.Fatal("blocked checkpoint reset or changed the held WAL")
				}
				// Checkpoint may legitimately write main/SHM; only the
				// subsequent quiescent-reader interval must be unchanged.
				before = afterCheckpoint
			}
			command := "resume"
			if mode == "checkpoint_cancel_after_page" {
				command = "cancel_after_page"
			}
			send(command)
		default:
			if strings.HasPrefix(line, "HMC276_CLOSE ") {
				t.Log(line)
			}
			if strings.HasPrefix(line, "HMC276_RESULT ") {
				if received {
					t.Fatal("duplicate terminal result")
				}
				received = true
				if e := json.Unmarshal([]byte(strings.TrimPrefix(line, "HMC276_RESULT ")), &result); e != nil {
					t.Fatal(e)
				}
			}
		}
	}
	err = cmd.Wait()
	waited = true
	if err != nil || scanner.Err() != nil || !admitted || !pinned || !received {
		t.Fatalf("helper lifecycle: err=%v scan=%v result=%+v transcript=%v stderr=%s", err, scanner.Err(), result, transcript, stderr.String())
	}
	after := hmc275ObserveSource(t, observers)
	for path, got := range after {
		want := before[path]
		if !got.generation.SameVersion(want.generation) {
			t.Logf("source lifecycle generation: %s bytes_equal=%v owner_equal=%v before=%+v after=%+v", filepath.Base(path), bytes.Equal(got.bytes, want.bytes), got.owner == want.owner, want.generation, got.generation)
		}
	}
	if expectNoGo {
		got, want := after[source+"-shm"], before[source+"-shm"]
		entries, e := os.ReadDir(out)
		if !result.SHMCloseChanged || got.generation.SameVersion(want.generation) || !bytes.Equal(got.bytes, want.bytes) || got.owner != want.owner {
			t.Fatalf("expected Darwin SHM-release no-go evidence not reproduced: %+v", result)
		}
		if e != nil || len(entries) != 0 || result.Path != "" || !strings.Contains(result.Error, "source generation changed while releasing shared-memory mapping") || result.Steps < 2 {
			t.Fatalf("known unsafe lifecycle was published: %+v entries=%v err=%v", result, entries, e)
		}
		if result.DeniedWrites != 0 {
			t.Fatalf("counterexample unexpectedly used explicit source writes: %+v", result)
		}
		// The sole changed generation is asserted above, not ignored or
		// reset to a newer baseline. Other held sources remain unchanged.
		delete(before, source+"-shm")
		hmc275AssertUnchanged(t, before)
		t.Log("NO-GO: readonly SHM release changed source generation after owner exit; completed private backup was rejected and removed; not a zero-effect capability PASS")
		return
	}
	hmc275AssertUnchanged(t, before)
	if result.DeniedWrites != 0 || result.Reads < 2 || result.SHMLocks == 0 {
		t.Fatalf("unexpected source I/O path: %+v", result)
	}
	if mode == "checkpoint_cancel_after_page" {
		entries, e := os.ReadDir(out)
		if e != nil || len(entries) != 0 || result.Path != "" || !strings.Contains(result.Error, "context canceled") || result.Steps != 1 {
			t.Fatalf("partial backup published after checkpoint: %+v entries=%v err=%v", result, entries, e)
		}
	} else if result.Error != "" || result.Path == "" || result.Steps < 2 {
		t.Fatalf("snapshot failed after lifecycle action: %+v transcript=%v", result, transcript)
	}
	if !strings.HasPrefix(mode, "owner_exit") {
		if r := checkpoint("TRUNCATE"); r != [3]int{0, 0, 0} {
			t.Fatalf("read locks leaked after helper exit: %v", r)
		}
		if info, e := observers[source+"-wal"].Stat(); e != nil || info.Size() != 0 {
			t.Fatalf("WAL did not actually truncate: %v %v", info, e)
		}
		execSQL("BEGIN; UPDATE first SET v=4; UPDATE second SET v=4; COMMIT;")
		var a, b int
		if e := writer.QueryRow("SELECT (SELECT v FROM first),(SELECT v FROM second)").Scan(&a, &b); e != nil || a != 4 || b != 4 {
			t.Fatalf("writer failed after WAL reset: %d %d %v", a, b, e)
		}
		t.Log("after helper exit: WAL truly truncated and new commit 4 is readable")
	}
	if result.Path != "" {
		snapshot, e := sql.Open("sqlite", "file:"+result.Path+"?mode=ro")
		if e != nil {
			t.Fatal(e)
		}
		defer snapshot.Close()
		var a, b int
		if e = snapshot.QueryRow("SELECT (SELECT v FROM first),(SELECT v FROM second)").Scan(&a, &b); e != nil || a != 2 || b != 2 {
			t.Fatalf("pinned commit did not survive owner/checkpoint lifecycle: %d %d %v", a, b, e)
		}
		t.Logf("snapshot remains commit 2/2; source quiescent windows unchanged; reads=%d locks=%d steps=%d", result.Reads, result.SHMLocks, result.Steps)
	}
}
