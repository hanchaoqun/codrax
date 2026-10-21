//go:build darwin

package hitraceconv

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

type hmc275CaptureResult struct {
	Path         string `json:"path"`
	Error        string `json:"error"`
	Reads        int    `json:"reads"`
	SHMLocks     int    `json:"shm_locks"`
	DeniedWrites int    `json:"denied_writes"`
	Steps        int    `json:"steps"`
}

func TestHMC275WALCaptureHelper(t *testing.T) {
	if os.Getenv("CODRAX_HMC275_WAL_HELPER") != "1" {
		t.Skip("isolated subprocess only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var observed *hmc275WALVFS
	var steps int
	cancelAfterStep := false
	path, err := hmc275Capture(ctx, os.Getenv("CODRAX_HMC275_SOURCE"), os.Getenv("CODRAX_HMC275_OUTPUT"), func(v *hmc275WALVFS) error {
		observed = v
		for _, f := range v.files {
			flags, err := unix.FcntlInt(f.Fd(), unix.F_GETFL, 0)
			if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY {
				return fmt.Errorf("source descriptor not readonly: %v", err)
			}
		}
		fmt.Println("HMC275_PINNED")
		s := bufio.NewScanner(os.Stdin)
		if !s.Scan() {
			return fmt.Errorf("controller closed before release")
		}
		if s.Text() == "cancel" {
			cancel()
		}
		cancelAfterStep = s.Text() == "cancel_after_page"
		return nil
	}, func(n int) {
		steps = n
		if cancelAfterStep {
			cancel()
		}
	})
	r := hmc275CaptureResult{Path: path, Steps: steps}
	if err != nil {
		r.Error = err.Error()
	}
	if observed != nil {
		r.Reads = observed.reads
		r.SHMLocks = observed.shmLocks
		r.DeniedWrites = observed.deniedWrites
	}
	b, _ := json.Marshal(r)
	fmt.Println("HMC275_RESULT " + string(b))
}

func TestHMC275ActiveWALHandleBoundSnapshot(t *testing.T) {
	for _, mode := range []string{"consistent", "cancel", "cancel_after_page", "replace_path"} {
		t.Run(mode, func(t *testing.T) {
			sourceDir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(sourceDir, "capture.db")
			writer, err := sql.Open("sqlite", source)
			if err != nil {
				t.Fatal(err)
			}
			writer.SetMaxOpenConns(1)
			defer writer.Close()
			mustExec := func(q string) {
				t.Helper()
				if _, err := writer.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			mustExec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; PRAGMA cache_size=1; CREATE TABLE first(v INTEGER, payload BLOB); CREATE TABLE second(v INTEGER); INSERT INTO first VALUES(1,zeroblob(262144)); INSERT INTO second VALUES(1); PRAGMA wal_checkpoint(TRUNCATE);")
			mustExec("BEGIN; UPDATE first SET v=2; UPDATE second SET v=2; COMMIT;")
			observers := make(map[string]*os.File)
			for _, suffix := range []string{"", "-wal", "-shm"} {
				f, e := os.Open(source + suffix)
				if e != nil {
					t.Fatal(e)
				}
				observers[source+suffix] = f
				// Closing any descriptor for an inode drops this process's POSIX
				// locks. Keep observation FDs until after writer.Close (defer).
				t.Cleanup(func() { _ = f.Close() })
			}
			initial := hmc275ObserveSource(t, observers)
			out := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHMC275WALCaptureHelper$", "-test.v")
			cmd.Env = append(os.Environ(), "CODRAX_HMC275_WAL_HELPER=1", "CODRAX_HMC275_SOURCE="+source, "CODRAX_HMC275_OUTPUT="+out)
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
			scanner := bufio.NewScanner(stdout)
			var transcript []string
			pinned := false
			var result hmc275CaptureResult
			var before map[string]hmc275SourceObservation
			for scanner.Scan() {
				line := scanner.Text()
				transcript = append(transcript, line)
				if line == "HMC275_PINNED" {
					pinned = true
					hmc275AssertUnchanged(t, initial)
					mustExec("BEGIN; UPDATE first SET v=3; UPDATE second SET v=3; COMMIT;")
					committedWAL, e := observers[source+"-wal"].Stat()
					if e != nil {
						t.Fatal(e)
					}
					mustExec("BEGIN; UPDATE first SET v=4, payload=zeroblob(1048576);")
					uncommittedWAL, e := observers[source+"-wal"].Stat()
					if e != nil {
						t.Fatal(e)
					}
					if uncommittedWAL.Size() <= committedWAL.Size() {
						t.Fatal("fixture did not spill a real uncommitted WAL tail")
					}
					t.Logf("uncommitted WAL tail physically spilled: %d -> %d bytes", committedWAL.Size(), uncommittedWAL.Size())
					if mode == "replace_path" { // Change namespace after the read transaction; held objects must win.
						for _, suffix := range []string{"", "-wal", "-shm"} {
							if err = os.Rename(source+suffix, source+suffix+".held"); err != nil {
								t.Fatal(err)
							}
						}
						if err = os.WriteFile(source, []byte("not the held database"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					before = hmc275ObserveSource(t, observers)
					command := "resume\n"
					if strings.HasPrefix(mode, "cancel") {
						command = mode + "\n"
					}
					if _, err = ioWriteStringHMC275(stdin, command); err != nil {
						t.Fatal(err)
					}
					_ = stdin.Close()
				}
				if strings.HasPrefix(line, "HMC275_RESULT ") {
					if err = json.Unmarshal([]byte(strings.TrimPrefix(line, "HMC275_RESULT ")), &result); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err = cmd.Wait(); err != nil {
				t.Fatalf("helper: %v stderr=%s transcript=%v", err, stderr.String(), transcript)
			}
			if !pinned {
				t.Fatalf("real read transaction not established: %v stderr=%s", transcript, stderr.String())
			}
			hmc275AssertUnchanged(t, before)
			if strings.HasPrefix(mode, "cancel") {
				entries, e := os.ReadDir(out)
				if e != nil || len(entries) != 0 || result.Path != "" || !strings.Contains(result.Error, "context canceled") {
					t.Fatalf("cancel published: %+v files=%v err=%v", result, entries, e)
				}
				if mode == "cancel_after_page" && result.Steps != 1 {
					t.Fatalf("did not cancel actual partial backup: %+v", result)
				}
				return
			}
			if result.Error != "" || result.Path == "" || result.Reads < 2 || result.SHMLocks == 0 || result.DeniedWrites != 0 {
				t.Fatalf("incomplete capture: %+v transcript=%v", result, transcript)
			}
			if mode == "replace_path" {
				b, e := os.ReadFile(source)
				if e != nil || string(b) != "not the held database" {
					t.Fatalf("replacement pathname was touched: %q %v", b, e)
				}
			}
			snapshot, e := sql.Open("sqlite", "file:"+result.Path+"?mode=ro")
			if e != nil {
				t.Fatal(e)
			}
			defer snapshot.Close()
			var a, b int
			if e = snapshot.QueryRow("SELECT (SELECT v FROM first),(SELECT v FROM second)").Scan(&a, &b); e != nil {
				t.Fatal(e)
			}
			if a != 2 || b != 2 {
				t.Fatalf("snapshot mixed commits or leaked uncommitted tail: %d %d", a, b)
			}
			t.Logf("real pinned transaction: first=%d second=%d; committed=3/uncommitted=4 excluded; held source unchanged; reads=%d shared-lock ops=%d", a, b, result.Reads, result.SHMLocks)
		})
	}
}

type hmc275SourceObservation struct {
	file       *os.File
	bytes      []byte
	generation filegeneration.Identity
	owner      [2]uint32
}

func hmc275ObserveSource(t *testing.T, observers map[string]*os.File) map[string]hmc275SourceObservation {
	t.Helper()
	r := make(map[string]hmc275SourceObservation)
	for path, f := range observers {
		info, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		b := make([]byte, info.Size())
		if _, err = f.ReadAt(b, 0); err != nil {
			t.Fatal(err)
		}
		id, err := filegeneration.FromFile(f)
		if err != nil {
			t.Fatal(err)
		}
		stat := info.Sys().(*syscall.Stat_t)
		r[path] = hmc275SourceObservation{file: f, bytes: b, generation: id, owner: [2]uint32{stat.Uid, stat.Gid}}
	}
	return r
}

func hmc275AssertUnchanged(t *testing.T, before map[string]hmc275SourceObservation) {
	t.Helper()
	for path, want := range before {
		info, err := want.file.Stat()
		if err != nil {
			t.Fatal(err)
		}
		b := make([]byte, info.Size())
		_, err = want.file.ReadAt(b, 0)
		if err != nil || !bytes.Equal(b, want.bytes) {
			t.Fatalf("capture changed source bytes before pin: %s %v", path, err)
		}
		id, err := filegeneration.FromFile(want.file)
		if err != nil || !id.SameVersion(want.generation) {
			t.Fatalf("capture changed source generation before pin: %s %v", path, err)
		}
		stat := info.Sys().(*syscall.Stat_t)
		if [2]uint32{stat.Uid, stat.Gid} != want.owner {
			t.Fatalf("capture changed owner before pin: %s", path)
		}
	}
}

func TestHMC275ActiveWALPrerequisitesFailClosed(t *testing.T) {
	for _, mode := range []string{"missing_shm", "symlink_wal", "fifo_wal", "no_live_owner"} {
		t.Run(mode, func(t *testing.T) {
			dir, e := filepath.EvalSymlinks(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			source := filepath.Join(dir, "source.db")
			writer, e := sql.Open("sqlite", source)
			if e != nil {
				t.Fatal(e)
			}
			writer.SetMaxOpenConns(1)
			defer writer.Close()
			if _, e = writer.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; CREATE TABLE x(v); INSERT INTO x VALUES(2);"); e != nil {
				t.Fatal(e)
			}
			expected := ""
			switch mode {
			case "missing_shm":
				if e = os.Rename(source+"-shm", source+"-shm.held"); e != nil {
					t.Fatal(e)
				}
				expected = "no such file"
			case "symlink_wal":
				if e = os.Rename(source+"-wal", source+"-wal.held"); e != nil {
					t.Fatal(e)
				}
				if e = os.Symlink(source+"-wal.held", source+"-wal"); e != nil {
					t.Fatal(e)
				}
				expected = "too many levels"
			case "fifo_wal":
				if e = os.Rename(source+"-wal", source+"-wal.held"); e != nil {
					t.Fatal(e)
				}
				if e = unix.Mkfifo(source+"-wal", 0600); e != nil {
					t.Fatal(e)
				}
				expected = "invalid or oversized"
			case "no_live_owner":
				// Byte copies are only a stale-SHM negative fixture, never a snapshot.
				copyDir, e := filepath.EvalSymlinks(t.TempDir())
				if e != nil {
					t.Fatal(e)
				}
				copyPath := filepath.Join(copyDir, "stale.db")
				for _, s := range []string{"", "-wal", "-shm"} {
					b, e := os.ReadFile(source + s)
					if e != nil {
						t.Fatal(e)
					}
					if e = os.WriteFile(copyPath+s, b, 0600); e != nil {
						t.Fatal(e)
					}
				}
				source = copyPath
				expected = "no live external SHM owner"
			}
			out := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHMC275WALCaptureHelper$", "-test.v")
			cmd.Env = append(os.Environ(), "CODRAX_HMC275_WAL_HELPER=1", "CODRAX_HMC275_SOURCE="+source, "CODRAX_HMC275_OUTPUT="+out)
			output, e := cmd.CombinedOutput()
			if e != nil {
				t.Fatalf("helper: %v %s", e, output)
			}
			if strings.Contains(string(output), "HMC275_PINNED") || !strings.Contains(string(output), expected) {
				t.Fatalf("wrong fail-closed boundary: %s", output)
			}
			entries, e := os.ReadDir(out)
			if e != nil || len(entries) != 0 {
				t.Fatalf("negative published: %v %v", entries, e)
			}
		})
	}
}

func TestHMC275ActiveWALRequiresIsolatedHelper(t *testing.T) {
	if os.Getenv("CODRAX_HMC275_WAL_HELPER") == "1" {
		t.Skip("parent only")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	out := t.TempDir()
	path, err := hmc275Capture(ctx, "unused", out, nil, nil)
	if path != "" || err == nil || !strings.Contains(err.Error(), "isolated helper") {
		t.Fatalf("unisolated source open allowed: %q %v", path, err)
	}
	entries, err := os.ReadDir(out)
	if err != nil || len(entries) != 0 {
		t.Fatal("unisolated attempt published")
	}
}

func ioWriteStringHMC275(w interface{ Write([]byte) (int, error) }, s string) (int, error) {
	return w.Write([]byte(s))
}
