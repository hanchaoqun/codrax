package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseSealedConversionPublicationPublishesPerfSidecarNoReplace(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("exact sealed-output publication is intentionally fail-closed on this platform")
	}
	parent := t.TempDir()
	finalPath := filepath.Join(parent, "capture.perf.data")
	target, err := prepareSealedConversionPublicationTarget(finalPath, ".codrax-perf-sidecar-*")
	if err != nil {
		t.Fatal(err)
	}
	cleaned := false
	defer func() {
		if !cleaned {
			_ = target.Cleanup()
		}
	}()
	if filepath.Base(target.StagingPath) != filepath.Base(finalPath) || target.StagingPath == finalPath {
		t.Fatalf("private staging did not preserve only the public basename: staging=%q final=%q", target.StagingPath, finalPath)
	}
	body := bytes.Repeat([]byte("sealed-hiperf-payload\n"), 64)
	if err := os.WriteFile(target.StagingPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := target.stagingDir.AdoptRegularChild(target.finalLeaf, true)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(finalPath); !os.IsNotExist(err) {
		t.Fatalf("public sidecar existed before publication: %v", err)
	}
	if err := publishSealedConversionFileNoReplace(context.Background(), target, source, ledger); err != nil {
		t.Fatalf("publish sealed perf sidecar: %v", err)
	}
	if len(ledger.created) != 1 || !ledger.created[0].authorityBound || !ledger.created[0].sealed {
		t.Fatalf("sealed publication did not enter the exact authority ledger: %+v", ledger.created)
	}
	got, err := os.ReadFile(finalPath)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("public sidecar was not first-visible as the complete payload: bytes=%d err=%v", len(got), err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	stagingRoot := target.stagingDir.Path()
	if err := target.Cleanup(); err != nil {
		t.Fatalf("cleanup private sidecar staging: %v", err)
	}
	cleaned = true
	if _, err := os.Lstat(stagingRoot); !os.IsNotExist(err) {
		t.Fatalf("private sidecar staging survived publication: %v", err)
	}
	if err := ledger.validateOwnedPaths(); err != nil {
		t.Fatalf("published sidecar lost its retained authority: %v", err)
	}
	if err := ledger.releaseOwnedAuthorities(); err != nil {
		t.Fatalf("release committed sidecar authority: %v", err)
	}
	got, err = os.ReadFile(finalPath)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("committed sidecar changed after authority release: bytes=%d err=%v", len(got), err)
	}
}

func TestReleaseSealedConversionPublicationCollisionPreservesExternalOwner(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("exact sealed-output publication is intentionally fail-closed on this platform")
	}
	parent := t.TempDir()
	finalPath := filepath.Join(parent, "capture.perf.data")
	target, err := prepareSealedConversionPublicationTarget(finalPath, ".codrax-perf-sidecar-race-*")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Cleanup()
	privateBody := []byte("private HIPERF payload")
	if err := os.WriteFile(target.StagingPath, privateBody, 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := target.stagingDir.AdoptRegularChild(target.finalLeaf, true)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	externalBody := []byte("external racing owner")
	if err := os.WriteFile(finalPath, externalBody, 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	err = publishSealedConversionFileNoReplace(context.Background(), target, source, ledger)
	if err == nil || !strings.Contains(err.Error(), "publish sealed conversion output") {
		t.Fatalf("expected typed sealed-output collision, got %v", err)
	}
	lowerError := strings.ToLower(err.Error())
	if strings.Contains(lowerError, "trace db") || strings.Contains(lowerError, "retained-db") || strings.Contains(lowerError, "retained-trace-db") {
		t.Fatalf("generic sidecar publication leaked retained DB diagnostics: %v", err)
	}
	if len(ledger.created) != 0 {
		t.Fatalf("failed collision publication registered an owner: %+v", ledger.created)
	}
	if err := source.Validate(); err != nil {
		t.Fatalf("collision consumed or weakened the private source authority: %v", err)
	}
	got, readErr := os.ReadFile(finalPath)
	if readErr != nil || !bytes.Equal(got, externalBody) {
		t.Fatalf("collision changed the external owner: got=%q err=%v", got, readErr)
	}
}

func TestReleaseSealedConversionPublicationRejectsForeignPrivateSource(t *testing.T) {
	parent := t.TempDir()
	finalPath := filepath.Join(parent, "capture.perf.data")
	target, err := prepareSealedConversionPublicationTarget(finalPath, ".codrax-perf-sidecar-target-*")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Cleanup()
	foreignDir, err := newPrivateConversionDir(parent, ".codrax-perf-sidecar-foreign-*")
	if err != nil {
		t.Fatal(err)
	}
	defer foreignDir.FinalizeCleanup()
	foreignPath, err := foreignDir.ChildPath(target.finalLeaf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreignPath, []byte("foreign private payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	foreign, err := foreignDir.AdoptRegularChild(target.finalLeaf, true)
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	err = publishSealedConversionFileNoReplace(context.Background(), target, foreign, ledger)
	if err == nil || !strings.Contains(err.Error(), "does not belong to its publication target") {
		t.Fatalf("foreign private source entered sealed publication target: %v", err)
	}
	if len(ledger.created) != 0 {
		t.Fatalf("foreign private source registered a public authority: %+v", ledger.created)
	}
	if _, err := os.Lstat(finalPath); !os.IsNotExist(err) {
		t.Fatalf("foreign private source reached the public path: %v", err)
	}
}

func TestReleaseSealedConversionPublicationStagingNamespaceCannotAliasPublicLeaf(t *testing.T) {
	parent := t.TempDir()
	for _, test := range []struct {
		name    string
		leaf    string
		pattern string
	}{
		{name: "exact", leaf: ".codrax-perf-" + strings.Repeat("a", 32) + ".stage", pattern: ".codrax-perf-*.stage"},
		{name: "case-folded", leaf: ".codrax-perf-" + strings.Repeat("A", 32) + ".stage", pattern: ".CODRAX-PERF-*.STAGE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			finalPath := filepath.Join(parent, test.leaf)
			_, err := prepareSealedConversionPublicationTarget(finalPath, test.pattern)
			if err == nil || !strings.Contains(err.Error(), "staging namespace can alias") {
				t.Fatalf("ambiguous staging/public namespace was accepted: %v", err)
			}
			if _, err := os.Lstat(finalPath); !os.IsNotExist(err) {
				t.Fatalf("ambiguous staging namespace made the public path visible: %v", err)
			}
		})
	}

	ordinaryFinal := filepath.Join(parent, "capture.perf.data")
	target, err := prepareSealedConversionPublicationTarget(ordinaryFinal, "capture.perf.data")
	if err != nil {
		t.Fatalf("a fixed prefix is still randomized and cannot alias its shorter final leaf: %v", err)
	}
	defer target.Cleanup()
	if target.StagingPath == ordinaryFinal {
		t.Fatalf("randomized staging root reused the public output path: %q", ordinaryFinal)
	}
	if _, err := os.Lstat(ordinaryFinal); !os.IsNotExist(err) {
		t.Fatalf("ordinary target made its public output visible during prepare: %v", err)
	}
}

func TestReleaseSealedConversionPublicationUsesSingleExactAuthority(t *testing.T) {
	generic := sourceGenerationFunctionBody(t, "retained_trace_db_publication.go", "publishSealedConversionFileNoReplace")
	genericHook := sourceGenerationFunctionBody(t, "retained_trace_db_publication.go", "publishSealedConversionFileNoReplaceWithValidation")
	retained := sourceGenerationFunctionBody(t, "retained_trace_db_publication.go", "publishOneRetainedTraceDBFile")
	wrapper := sourceGenerationFunctionBody(t, "retained_trace_db_publication.go", "publishSealedConversionFileWithBinding")
	core := sourceGenerationFunctionBody(t, "retained_trace_db_publication.go", "publishSealedConversionFileWithBindingValidation")
	if strings.Count(generic, "publishSealedConversionFileNoReplaceWithValidation(") != 1 ||
		strings.Count(genericHook, "publishSealedConversionFileWithBindingValidation(") != 1 {
		t.Fatalf("generic publication regained a second authority throat:\n%s\n%s", generic, genericHook)
	}
	for name, body := range map[string]string{"retained": retained} {
		if strings.Count(body, "publishSealedConversionFileWithBinding(") != 1 {
			t.Fatalf("%s publication regained a second authority throat:\n%s", name, body)
		}
	}
	if strings.Count(wrapper, "publishSealedConversionFileWithBindingValidation(") != 1 {
		t.Fatalf("compatibility publication wrapper regained a second authority throat:\n%s", wrapper)
	}
	for _, required := range []string{"publishSealedConversionFilePlatform(", "ledger.recordSealedAuthority("} {
		if strings.Count(core, required) != 1 {
			t.Fatalf("exact publication core missing singleton %q:\n%s", required, core)
		}
	}
	for _, forbidden := range []string{"recordIdentity(", "sealOwnedPath(", "publishConversionFileNoReplace(", "os.Rename(", "os.Link("} {
		if strings.Contains(generic, forbidden) || strings.Contains(core, forbidden) {
			t.Fatalf("generic sealed publication introduced weak path primitive %q", forbidden)
		}
	}
	prepare := sourceGenerationFunctionBody(t, "retained_trace_db_publication.go", "prepareSealedConversionPublicationTarget")
	for _, required := range []string{"sealedConversionStagingPatternAliasesLeaf(pattern, leaf)", "filepath.Dir(absoluteFinal)", "newPrivateConversionDir(parent, pattern)", "stagingDir.ChildPath(leaf)"} {
		if !strings.Contains(prepare, required) {
			t.Fatalf("sealed publication target is not rooted in its final parent: missing %q\n%s", required, prepare)
		}
	}
}

func TestReleaseRetainedTraceDBPublicationErrorWordingCompatibility(t *testing.T) {
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	err = publishOneRetainedTraceDBFile(context.Background(), traceStreamerDBTarget{}, nil, "", ledger)
	if got, want := err.Error(), "sealed retained trace DB source is incomplete"; got != want {
		t.Fatalf("retained trace DB source error changed: got=%q want=%q", got, want)
	}
	_, err = newRetainedTraceDBPublication(nil, publishedConversionFilePlatformState{}, sealedConversionPublicationRetainedTraceDB, "", "", "", 0)
	if got, want := err.Error(), "retained trace DB publication authority is incomplete"; got != want {
		t.Fatalf("retained trace DB authority error changed: got=%q want=%q", got, want)
	}
}
