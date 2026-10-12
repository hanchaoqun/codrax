package dataquery

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestHMC228NamedPrivateCopyCannotCollideWithRepositoryInput(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	input := filepath.Join(outside, "sample.csv")
	if err := os.WriteFile(input, []byte("value\n7\n8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(input))
	rel := filepath.ToSlash(filepath.Join("_named", fmt.Sprintf("%x", digest), "sample.csv"))
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), []byte("value\n99\n99\n"), 0600); err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(input)
	repoQuoted, _ := json.Marshal(rel)
	for _, paths := range [][]string{{input, rel}, {rel, input}, {input, "_named"}} {
		plan := TaskPlan{Status: "ready", InputPaths: paths,
			Script:         "a=sum(int(row['value']) for row in csv_rows(" + string(quoted) + "))\nb=sum(int(row['value']) for row in csv_rows(" + string(repoQuoted) + "))\nemit_result(str(a)+'/'+str(b))",
			OutputContract: OutputContract{Format: OutputPlainSingleLine}}
		runner := Runner{RepoRoot: root, TempRoot: t.TempDir(), NamedInputs: NewNamedInputs([]string{input}), MaxTotalBytes: int64(len("value\n7\n8\n") + len("value\n99\n99\n"))}
		result, err := runner.Run(context.Background(), plan)
		if err != nil || result.Answer != "15/198" || len(result.ConsumedPaths) != 2 {
			t.Fatalf("named/repository source collision for %v: %+v %v", paths, result, err)
		}
		runner.MaxTotalBytes--
		if _, err := runner.Run(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "input total exceeds") {
			t.Fatalf("ordinary and named copies escaped their shared byte budget: %v", err)
		}
	}
}

func TestHMC228NamedRepositoryAbsolutePathKeepsItsPublicIdentity(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "sample.csv")
	if err := os.WriteFile(input, []byte("value\n7\n8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(input)
	plan := TaskPlan{Status: "ready", InputPaths: []string{input}, Script: "emit_result(sum(int(row['value']) for row in csv_rows(" + string(quoted) + ")))", OutputContract: OutputContract{Format: OutputPlainSingleLine}}
	result, err := (Runner{RepoRoot: root, TempRoot: t.TempDir(), NamedInputs: NewNamedInputs([]string{input})}).Run(context.Background(), plan)
	if err != nil || result.Answer != "15" || len(result.ConsumedPaths) != 1 || result.ConsumedPaths[0] != input {
		t.Fatalf("current named absolute repo source lost its identity: %+v %v", result, err)
	}
}

func TestHMC228UnknownInspectionDoesNotClaimConsumption(t *testing.T) {
	result, err := (ActionRunner{RepoRoot: t.TempDir(), TempRoot: t.TempDir()}).Run(context.Background(), TaskPlan{Status: "ready", Actions: []DataAction{{ID: "inspect", Kind: DataActionInspectMaterial, InputPaths: []string{"missing.data"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ConsumedPaths) != 0 {
		t.Fatalf("unknown input claimed consumed: %v", result.ConsumedPaths)
	}
}

func TestHMC228NamedExternalDataExecutesWithoutParentAuthority(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	input := filepath.Join(outside, "sample.csv")
	other := filepath.Join(outside, "other.csv")
	for _, p := range []string{input, other} {
		if err := os.WriteFile(p, []byte("value\n7\n8\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	named := NewNamedInputs([]string{input})
	quoted, _ := json.Marshal(input)
	plan := TaskPlan{Status: "ready", InputPaths: []string{input}, Script: "rows=csv_rows(" + string(quoted) + ")\nemit_result(sum(int(row['value']) for row in rows))", OutputContract: OutputContract{Format: OutputPlainSingleLine}}
	runner := Runner{RepoRoot: root, TempRoot: t.TempDir(), NamedInputs: named}
	result, err := runner.Run(context.Background(), plan)
	if err != nil || result.Answer != "15" || len(result.ConsumedPaths) != 1 || result.ConsumedPaths[0] != input {
		t.Fatalf("actual external CSV script: %+v %v", result, err)
	}
	for _, bad := range []string{other, outside, filepath.Join(t.TempDir(), "sample.csv")} {
		badPlan := plan
		badPlan.InputPaths = []string{bad}
		if _, err := runner.Run(context.Background(), badPlan); err == nil {
			t.Fatalf("unnamed source authorized: %s", bad)
		}
	}
	inspect := TaskPlan{Actions: []DataAction{{ID: "inspect", Kind: DataActionInspectMaterial, InputPaths: []string{input}}}}
	got, err := (ActionRunner{RepoRoot: root, TempRoot: t.TempDir(), NamedInputs: named}).Run(context.Background(), inspect)
	if err != nil || len(got.ConsumedPaths) != 1 || got.ConsumedPaths[0] != input {
		t.Fatalf("actual external inspection: %+v %v", got, err)
	}
	var decoded NamedInputs
	encoded, _ := json.Marshal(named)
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	runner.NamedInputs = &decoded
	if _, err := runner.Run(context.Background(), plan); err == nil {
		t.Fatal("JSON revived external input permission")
	}
}

func TestHMC228NamedGenerationIsPerSelectedRun(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	paths := []string{filepath.Join(outside, "a.csv"), filepath.Join(outside, "b.csv")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("value\n7\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	named := NewNamedInputs(paths)
	if err := os.WriteFile(paths[0], []byte("value\n8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := func(path string) TaskPlan {
		return TaskPlan{Actions: []DataAction{{ID: "inspect", Kind: DataActionInspectMaterial, InputPaths: []string{path}}}}
	}
	runner := ActionRunner{RepoRoot: root, TempRoot: t.TempDir(), NamedInputs: named}
	if got, err := runner.Run(context.Background(), plan(paths[1])); err != nil || len(got.ConsumedPaths) != 1 {
		t.Fatalf("unused changed side blocked healthy source: %+v %v", got, err)
	}
	if got, err := runner.Run(context.Background(), plan(paths[0])); err == nil || len(got.ConsumedPaths) > 0 {
		t.Fatalf("changed selected side consumed: %+v %v", got, err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := runner.Run(context.Background(), plan(paths[1]))
			if err != nil || len(got.ConsumedPaths) != 1 {
				t.Errorf("shared immutable named inputs: %+v %v", got, err)
			}
		}()
	}
	wg.Wait()
	alias := filepath.Join(outside, "alias.csv")
	if err := os.Symlink(paths[1], alias); err != nil {
		t.Fatal(err)
	}
	aliasInputs := NewNamedInputs([]string{alias})
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(paths[0], alias); err != nil {
		t.Fatal(err)
	}
	if _, _, err := aliasInputs.resolve(root, alias); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatal("retargeted alias survived generation check", err)
	}
}
