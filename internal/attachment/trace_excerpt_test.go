package attachment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestTraceExcerptCompleteLineBoundaries(t *testing.T) {
	parent := "head\r\n业务\r\nworker\r\ntail"
	for _, tc := range []struct {
		name               string
		start, end         int
		want               string
		lineStart, lineEnd int
	}{
		{"whole", 0, len(parent), parent, 1, 4},
		{"middle", len("head\r\n"), len("head\r\n业务\r\nworker\r\n"), "业务\r\nworker\r\n", 2, 3},
		{"clipped utf8 and tail", len("head\r\n") + 1, len(parent) - 1, "worker\r\n", 3, 3},
		{"last line without newline", strings.Index(parent, "tail"), len(parent), "tail", 4, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := NewTraceExcerpt(context.Background(), parent, nil, tc.start, tc.end)
			if err != nil {
				t.Fatal(err)
			}
			raw, s, err := v.Resolve(context.Background(), parent, nil)
			if err != nil || raw != tc.want || s.LineStart != tc.lineStart || s.LineEnd != tc.lineEnd || s.LineCoordinates != "parent_preview" || s.ByteStart < tc.start || s.ByteEnd > tc.end {
				t.Fatalf("raw=%q scope=%+v err=%v", raw, s, err)
			}
			s.ByteStart = 9999
			if _, _, err := v.Resolve(context.Background(), parent, nil); err != nil {
				t.Fatal("scope copy mutated receipt", err)
			}
		})
	}
	for _, r := range [][2]int{{-1, 1}, {0, 0}, {4, 3}, {0, len(parent) + 1}, {1, 3}, {6, 7}} {
		if _, err := NewTraceExcerpt(context.Background(), parent, nil, r[0], r[1]); err == nil {
			t.Fatalf("accepted invalid/partial-only range %v", r)
		}
	}
}

func TestTraceExcerptCannotRebindOrRestore(t *testing.T) {
	parent := "first\nsecond\n"
	v, err := NewTraceExcerpt(context.Background(), parent, nil, 6, len(parent))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := v.Resolve(context.Background(), "other\nsecond\n", nil); err == nil {
		t.Fatal("rebound parent")
	}
	bytes, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var restored TraceExcerpt
	if err := json.Unmarshal(bytes, &restored); err != nil {
		t.Fatal(err)
	}
	if _, _, err := restored.Resolve(context.Background(), parent, nil); err == nil {
		t.Fatal("JSON restored view")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := v.Resolve(ctx, parent, nil); err != context.Canceled {
		t.Fatalf("cancel=%v", err)
	}
}

func TestTraceExcerptValidatesEveryParentMaterialMember(t *testing.T) {
	for _, changed := range []string{"source", "converted", "member"} {
		t.Run(changed, func(t *testing.T) {
			dir := t.TempDir()
			ids := map[string]filegeneration.Identity{}
			for _, name := range []string{"source", "converted", "member"} {
				p := filepath.Join(dir, name)
				if err := os.WriteFile(p, []byte("original\n"), 0600); err != nil {
					t.Fatal(err)
				}
				id, err := filegeneration.FromPath(p)
				if err != nil {
					t.Fatal(err)
				}
				ids[p] = id
			}
			parent := "# codrax-source: " + filepath.Join(dir, "converted") + "\nrow one\nrow two\n"
			m, err := BindTraceMaterial(filepath.Join(dir, "source"), filepath.Join(dir, "converted"), parent, ids)
			if err != nil {
				t.Fatal(err)
			}
			v, err := NewTraceExcerpt(context.Background(), parent, m, strings.Index(parent, "row one"), len(parent))
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := v.Resolve(context.Background(), parent, nil); err == nil {
				t.Fatal("dropped material")
			}
			other, err := BindTraceMaterial(filepath.Join(dir, "source"), filepath.Join(dir, "converted"), parent, ids)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := v.Resolve(context.Background(), parent, other); err == nil {
				t.Fatal("rebound equivalent but different receipt")
			}
			p := filepath.Join(dir, changed)
			st, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("modified\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(p, st.ModTime(), st.ModTime()); err != nil {
				t.Fatal(err)
			}
			if _, _, err := v.Resolve(context.Background(), parent, m); err == nil {
				t.Fatal("accepted changed member")
			}
		})
	}
}

func TestTraceExcerptPreparedEOFNeedsTypedCompleteness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.sys")
	body := "first event\nsecond event without newline"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := filegeneration.FromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]filegeneration.Identity{path: id}
	header := "# codrax-source: " + path + "\n"
	for _, complete := range []bool{false, true} {
		preview := header + body
		var m *TraceMaterial
		if complete {
			m, err = BindCompleteTextTraceMaterial(path, preview, ids)
		} else {
			preview = header + "first event\nsecond event with"
			m, err = BindTraceMaterial(path, path, preview, ids)
		}
		if err != nil {
			t.Fatal(err)
		}
		v, err := NewTraceExcerpt(context.Background(), preview, m, len(header), len(preview))
		if err != nil {
			t.Fatal(err)
		}
		raw, _, err := v.Resolve(context.Background(), preview, m)
		if err != nil {
			t.Fatal(err)
		}
		want := "first event\n"
		if complete {
			want = body
		}
		if raw != want {
			t.Fatalf("complete=%v got=%q want=%q", complete, raw, want)
		}
	}
}
