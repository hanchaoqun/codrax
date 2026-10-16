package outputdump

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitReportExternalChangeHMC251(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "created_after_registration", true: "edited_after_registration"}[existing], func(t *testing.T) {
			dir := t.TempDir()
			md := filepath.Join(dir, "report.md")
			html := filepath.Join(dir, "report.html")
			if existing {
				if err := os.WriteFile(md, []byte("old report"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			withExplicitReport(t, ExplicitReport{MarkdownPath: md, HTMLPath: html, SuppressDefaultDir: true})
			if err := os.WriteFile(md, []byte("other author's new report"), 0o600); err != nil {
				t.Fatal(err)
			}
			WriteResult(explicitReportArgs(filepath.Join(dir, "disabled")))
			if got := string(readFileOrFatal(t, md)); got != "other author's new report" {
				t.Errorf("public WriteResult replaced an externally changed destination: %q", got)
			}
			writes := ExplicitReportWrites()
			if len(writes) != 2 || writes[0].Err == nil || writes[1].Err != nil {
				t.Errorf("conflicted markdown and independent HTML statuses: %+v", writes)
			}
			if _, err := os.Stat(html); err != nil {
				t.Errorf("independent HTML must still be delivered: %v", err)
			}
		})
	}
}

func TestExplicitReportAtomicFailureKeepsOldBytesHMC251(t *testing.T) {
	for _, phase := range []string{"write", "short_write", "sync", "close", "rename"} {
		t.Run(phase, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "report.md")
			if err := os.WriteFile(path, []byte("old report survives"), 0o600); err != nil {
				t.Fatal(err)
			}
			withExplicitReport(t, ExplicitReport{MarkdownPath: path, SuppressDefaultDir: true})
			p := explicitReportSnapshot().publication
			boom := errors.New("injected " + phase)
			switch phase {
			case "write":
				p.ops.write = func(f *os.File, body []byte) (int, error) {
					n, _ := f.Write(body[:3])
					return n, boom
				}
			case "short_write":
				p.ops.write = func(f *os.File, body []byte) (int, error) { return f.Write(body[:3]) }
			case "sync":
				p.ops.sync = func(*os.File) error { return boom }
			case "close":
				p.ops.close = func(*os.File) error { return boom }
			case "rename":
				p.ops.rename = func(*os.Root, string, string) error { return boom }
			}
			WriteResult(explicitReportArgs(""))
			if got := string(readFileOrFatal(t, path)); got != "old report survives" {
				t.Fatalf("%s failure changed old report: %q", phase, got)
			}
			writes := ExplicitReportWrites()
			if len(writes) != 1 || writes[0].Err == nil || writes[0].Published || writes[0].SHA256 != "" || writes[0].Bytes != 0 || writes[0].Previous != nil {
				t.Fatalf("failure minted committed receipt: %+v", writes)
			}
			if phase == "short_write" && !errors.Is(writes[0].Err, io.ErrShortWrite) {
				t.Fatalf("short write was hidden: %v", writes[0].Err)
			}
			assertNoReportStageHMC251(t, dir)
		})
	}
}

func assertNoReportStageHMC251(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".codrax-report-") {
			t.Errorf("transaction leaked staging file %q", entry.Name())
		}
	}
}

func TestExplicitReportReadOnlyRegistrationAndRepeatHMC251(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-created", "report.md")
	withExplicitReport(t, ExplicitReport{MarkdownPath: path, SuppressDefaultDir: true})
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("registration must not create parent directories: %v", err)
	}
	a := explicitReportArgs("")
	WriteResult(a)
	a.Answer = "a new finalized answer"
	WriteResult(a)
	writes := ExplicitReportWrites()
	if len(writes) != 2 || writes[0].Err != nil || writes[1].Err != nil || !writes[0].Published || !writes[1].Published {
		t.Fatalf("own successful publication did not update expected version: %+v", writes)
	}
	if writes[0].Previous == nil || writes[0].Previous.Exists || writes[1].Previous.SHA256 != writes[0].SHA256 {
		t.Fatalf("wrong old version receipts: %+v", writes)
	}
	if string(readFileOrFatal(t, path)) != BuildBody(a) {
		t.Fatal("second explicit overwrite must stay permitted")
	}
	writes[1].Previous.SHA256 = "mutated returned snapshot"
	if ExplicitReportWrites()[1].Previous.SHA256 != writes[0].SHA256 {
		t.Fatal("receipt snapshots share mutable old-version pointers")
	}
	assertNoReportStageHMC251(t, filepath.Dir(path))
}

func TestExplicitReportVersionAndAliasChangesHMC251(t *testing.T) {
	for _, change := range []string{"same_bytes_replaced", "changed_during_staging", "symlink_retargeted"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "report.md")
			if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			requested := path
			if change == "symlink_retargeted" {
				requested = filepath.Join(dir, "link.md")
				if err := os.Symlink(path, requested); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			withExplicitReport(t, ExplicitReport{MarkdownPath: requested, SuppressDefaultDir: true})
			want := "original"
			switch change {
			case "same_bytes_replaced":
				replacement := filepath.Join(dir, "replacement")
				if err := os.WriteFile(replacement, []byte(want), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			case "changed_during_staging":
				want = "external edit while staging"
				explicitReportSnapshot().publication.ops.write = func(f *os.File, body []byte) (int, error) {
					if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
						return 0, err
					}
					return f.Write(body)
				}
			case "symlink_retargeted":
				other := filepath.Join(dir, "other.md")
				if err := os.WriteFile(other, []byte("other report"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(requested); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, requested); err != nil {
					t.Fatal(err)
				}
			}
			WriteResult(explicitReportArgs(""))
			if got := string(readFileOrFatal(t, path)); got != want {
				t.Fatalf("changed target overwritten: %q", got)
			}
			writes := ExplicitReportWrites()
			if len(writes) != 1 || writes[0].Err == nil || writes[0].Published {
				t.Fatalf("unverified target version accepted: %+v", writes)
			}
			if change == "symlink_retargeted" && string(readFileOrFatal(t, requested)) != "other report" {
				t.Fatal("retargeted symlink target was modified")
			}
			assertNoReportStageHMC251(t, dir)
		})
	}
}

func TestExplicitReportSymlinkReferentAndHardlinkHMC251(t *testing.T) {
	for _, kind := range []string{"symlink_existing", "symlink_missing", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "target.md")
			if kind != "symlink_missing" {
				if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			alias := filepath.Join(dir, "report.md")
			link := os.Symlink
			if kind == "hardlink" {
				link = os.Link
			}
			if err := link(target, alias); err != nil {
				t.Skipf("link unavailable: %v", err)
			}
			withExplicitReport(t, ExplicitReport{MarkdownPath: alias, SuppressDefaultDir: true})
			a := explicitReportArgs("")
			WriteResult(a)
			writes := ExplicitReportWrites()
			if len(writes) != 1 {
				t.Fatalf("missing write result: %+v", writes)
			}
			if kind == "hardlink" {
				if writes[0].Err == nil || writes[0].Published || string(readFileOrFatal(t, alias)) != "old" || string(readFileOrFatal(t, target)) != "old" {
					t.Fatalf("hardlinked destination must fail without changing either alias: %+v", writes)
				}
				return
			}
			info, err := os.Lstat(alias)
			if err != nil || info.Mode()&os.ModeSymlink == 0 || writes[0].Err != nil || !writes[0].Published {
				t.Fatalf("authorized symlink must retain link and publish referent: %v %+v", err, writes)
			}
			if string(readFileOrFatal(t, target)) != BuildBody(a) {
				t.Fatal("symlink referent not updated")
			}
			if kind == "symlink_existing" {
				info, err := os.Stat(target)
				if err != nil || info.Mode().Perm() != 0o600 {
					t.Fatalf("existing confidential report permissions changed: %v %v", info, err)
				}
			}
			assertNoReportStageHMC251(t, dir)
		})
	}
}

func TestExplicitReportInternalCancellationBoundaryHMC251(t *testing.T) {
	for _, phase := range []string{"before", "staging", "after_commit"} {
		t.Run(phase, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "report.md")
			if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			withExplicitReport(t, ExplicitReport{MarkdownPath: path, SuppressDefaultDir: true})
			r := explicitReportSnapshot()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch phase {
			case "before":
				cancel()
			case "staging":
				r.publication.ops.write = func(f *os.File, data []byte) (int, error) { cancel(); return f.Write(data) }
			case "after_commit":
				r.publication.ops.rename = func(root *os.Root, from, to string) error {
					err := root.Rename(from, to)
					cancel()
					return err
				}
			}
			// The CLI currently calls this bounded in-memory write with a
			// background context. This test claims only the internal boundary.
			got := publishExplicitReport(ctx, r, "markdown", path, []byte("new"))
			if phase == "after_commit" {
				if got.Err != nil || !got.Published || string(readFileOrFatal(t, path)) != "new" {
					t.Fatalf("late cancellation must not undo committed publication: %+v", got)
				}
			} else if !errors.Is(got.Err, context.Canceled) || got.Published || string(readFileOrFatal(t, path)) != "old" {
				t.Fatalf("pre-publication cancellation must retain old report: %+v", got)
			}
			assertNoReportStageHMC251(t, dir)
		})
	}
}

func TestExplicitReportJSONAndRetiredRegistrationNoAuthorityHMC251(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	withExplicitReport(t, ExplicitReport{MarkdownPath: path, SuppressDefaultDir: true})
	original := explicitReportSnapshot()
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored ExplicitReport
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	got := publishExplicitReport(context.Background(), restored, "markdown", path, []byte("forged"))
	if got.Err == nil || got.Published {
		t.Fatal("JSON paths restored private write authority")
	}
	SetExplicitReport(ExplicitReport{})
	got = publishExplicitReport(context.Background(), original, "markdown", path, []byte("retired"))
	if got.Err == nil || got.Published {
		t.Fatal("retired registration retained write authority")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unregistered publication touched destination: %v", err)
	}
}

func TestExplicitReportPublicationReceiptHMC251(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "report.md")
	if err := os.WriteFile(md, []byte("old report"), 0o600); err != nil {
		t.Fatal(err)
	}
	withExplicitReport(t, ExplicitReport{MarkdownPath: md, SuppressDefaultDir: true})
	a := explicitReportArgs("")
	WriteResult(a)
	writes := ExplicitReportWrites()
	if len(writes) != 1 || writes[0].Err != nil {
		t.Fatalf("explicit replacement failed: %+v", writes)
	}
	// Inspect the public receipt's JSON rather than inventing write authority
	// from it. Existing code compiles and visibly lacks these delivery facts.
	encoded, err := json.Marshal(writes[0])
	if err != nil {
		t.Fatal(err)
	}
	var receipt map[string]any
	if err := json.Unmarshal(encoded, &receipt); err != nil {
		t.Fatal(err)
	}
	body := readFileOrFatal(t, md)
	digest := sha256.Sum256(body)
	if receipt["published"] != true || receipt["bytes"] != float64(len(body)) || receipt["sha256"] != hex.EncodeToString(digest[:]) {
		t.Errorf("public write receipt must describe committed bytes only: %s", encoded)
	}
	previous, ok := receipt["previous"].(map[string]any)
	oldDigest := sha256.Sum256([]byte("old report"))
	if !ok || previous["exists"] != true || previous["sha256"] != hex.EncodeToString(oldDigest[:]) {
		t.Errorf("old target version missing from receipt: %s", encoded)
	}
	if string(body) != BuildBody(a) {
		t.Fatal("explicit report no longer uses exact final-answer bytes")
	}
}
