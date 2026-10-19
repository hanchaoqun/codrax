package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHMC264ZIPRoutingPublicCandidatesNotReadiness(t *testing.T) {
	for _, names := range [][]string{{"采样/捕获.data"}, {"采样/捕获.data", "other/无扩展名"}} {
		t.Run(names[0]+string(rune('0'+len(names))), func(t *testing.T) {
			root, anchor := t.TempDir(), t.TempDir()
			input := filepath.Join(root, "普通资料.zip")
			var members []traceArchiveTestMember
			for _, name := range names {
				members = append(members, traceArchiveTestMember{name: name, body: traceArchiveTestBuiltinBody()})
			}
			before := traceArchiveTestZIP(t, input, members...)
			got := ProbeInputRoutingCapability(context.Background(), input, anchor)
			if got.Container != "zip" || got.NativeReader != "trace_query" || got.Status != "archive_candidates" {
				t.Fatalf("safe known ZIP must expose navigation, not readiness: %+v", got)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var wire struct {
				ArchiveCandidates struct {
					Members           []string `json:"members"`
					CandidateCount    int      `json:"candidate_count"`
					MembersComplete   bool     `json:"members_complete"`
					SelectionRequired bool     `json:"selection_required"`
				} `json:"archive_candidates"`
			}
			if err := json.Unmarshal(encoded, &wire); err != nil {
				t.Fatal(err)
			}
			inventory := wire.ArchiveCandidates
			if !reflect.DeepEqual(inventory.Members, names) || inventory.CandidateCount != len(names) || !inventory.MembersComplete || inventory.SelectionRequired != (len(names) > 1) {
				t.Fatalf("exact inventory lost or selected implicitly: %s", encoded)
			}
			after, err := os.ReadFile(input)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("navigation changed source bytes: %v", err)
			}
			for _, directory := range []string{root, anchor} {
				entries, err := os.ReadDir(directory)
				want := 0
				if directory == root {
					want = 1
				}
				if err != nil || len(entries) != want {
					t.Fatalf("navigation extracted or wrote files: %s: %v %v", directory, entries, err)
				}
			}
		})
	}
}
