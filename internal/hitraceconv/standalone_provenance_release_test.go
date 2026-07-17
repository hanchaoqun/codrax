package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type standaloneProvenanceReleaseFixture struct {
	inputPath string
	artifacts []Artifact
	ledger    *conversionFileLedger
}

func newStandaloneProvenanceReleaseFixture(t *testing.T, payloads ...[]byte) standaloneProvenanceReleaseFixture {
	t.Helper()
	if len(payloads) == 0 {
		t.Fatal("standalone provenance fixture requires at least one payload")
	}
	dir := t.TempDir()
	body := make([]byte, 0)
	for _, payload := range payloads {
		body = append(body, syntheticStandaloneProfilerBlock(
			profilerDataTypeHiperf, "hiperf-plugin", "1.0", payload,
		)...)
	}
	inputPath := filepath.Join(dir, "standalone-source.htrace")
	if err := os.WriteFile(inputPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(inputPath)
	if unavailableConversionInputAuthority(t, err) {
		return standaloneProvenanceReleaseFixture{}
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := authority.Close(); closeErr != nil {
			t.Errorf("close standalone input authority: %v", closeErr)
		}
	})
	inventory, err := findStandaloneSegmentsFromInput(context.Background(), authority)
	if err != nil {
		t.Fatalf("inspect authenticated standalone layout: %v", err)
	}
	ledger, err := newConversionFileLedgerForAuthority(authority)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cleanupErr := ledger.cleanup(); cleanupErr != nil {
			t.Errorf("cleanup standalone publications: %v", cleanupErr)
		}
	})
	allArtifacts, _, _, err := extractStandaloneArtifactsWithOptionsAndLedger(
		context.Background(),
		Options{InputPath: inputPath, DisablePerfAdapter: true, PerfParser: "raw"},
		inventory,
		filepath.Join(dir, "standalone-source.systrace"),
		standaloneExtractOptions{GeneratePerfTrace: true},
		ledger,
	)
	if err != nil {
		t.Fatalf("extract receipt-backed standalone artifacts: %v", err)
	}
	rawArtifacts := make([]Artifact, 0, len(payloads))
	for _, artifact := range allArtifacts {
		if artifact.Type == ArtifactPerfData {
			rawArtifacts = append(rawArtifacts, artifact)
		}
	}
	if len(rawArtifacts) != len(payloads) {
		t.Fatalf("raw standalone artifacts=%d want=%d: %+v", len(rawArtifacts), len(payloads), allArtifacts)
	}
	return standaloneProvenanceReleaseFixture{
		inputPath: inputPath,
		artifacts: rawArtifacts,
		ledger:    ledger,
	}
}

func measuredStandaloneReleaseArtifact(t testing.TB, artifact Artifact) (int64, string) {
	t.Helper()
	body, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	return int64(len(body)), hex.EncodeToString(digest[:])
}

func standaloneReleaseAuthoritySegment(
	t testing.TB,
	fixture standaloneProvenanceReleaseFixture,
	artifact Artifact,
) standaloneSegment {
	t.Helper()
	receipt, ok := fixture.ledger.standaloneSourceReceiptForArtifactPath(artifact.Path)
	if !ok {
		t.Fatalf("standalone artifact has no independent ledger receipt authority: %+v", artifact)
	}
	return receipt.segment
}

func TestStandaloneLedgerReceiptAuthorityBaseline(t *testing.T) {
	fixture := newStandaloneProvenanceReleaseFixture(t, syntheticRawPerfData())
	if len(fixture.artifacts) == 0 {
		return
	}
	artifact := cloneArtifact(fixture.artifacts[0])
	published, ok := fixture.ledger.standaloneSourceReceiptForArtifactPath(artifact.Path)
	if !ok {
		t.Fatal("legal standalone artifact has no ledger-owned source receipt")
	}
	bindingPath := published.segment.BindingPath
	if !filepath.IsAbs(bindingPath) || filepath.Clean(bindingPath) != bindingPath ||
		published.segment.ArtifactPath != artifact.Path {
		t.Fatalf("ledger source receipt path binding is not exact: %+v", published.segment)
	}
	createdIndex, ok := fixture.ledger.byPath[bindingPath]
	if !ok || createdIndex < 0 || createdIndex >= len(fixture.ledger.created) {
		t.Fatalf("ledger source receipt has no created generation: %q", bindingPath)
	}
	created := fixture.ledger.created[createdIndex]
	if created.path != bindingPath || created.standaloneSource == nil ||
		!published.publishedIdentity.SameVersion(created.sealedIdentity) ||
		!created.standaloneSource.publishedIdentity.SameVersion(created.sealedIdentity) {
		t.Fatalf("ledger receipt is not bound to its sealed created generation: %+v", created)
	}
	if artifact.standaloneReceipt == nil || !reflect.DeepEqual(*artifact.standaloneReceipt, published.segment) {
		t.Fatalf("legal artifact private receipt does not match independent authority: artifact=%+v authority=%+v",
			artifact.standaloneReceipt, published.segment)
	}
	artifact.standaloneReceipt.Offset++
	artifact.standaloneReceipt.BindingPath = "forged-relative"
	reloaded, ok := fixture.ledger.standaloneSourceReceiptForArtifactPath(artifact.Path)
	if !ok || !reflect.DeepEqual(reloaded.segment, published.segment) ||
		reloaded.segment.Offset == artifact.standaloneReceipt.Offset ||
		reloaded.segment.BindingPath == artifact.standaloneReceipt.BindingPath {
		t.Fatalf("artifact receipt mutation contaminated ledger authority: before=%+v after=%+v artifact=%+v",
			published.segment, reloaded.segment, artifact.standaloneReceipt)
	}
}

func TestStandaloneLedgerCommitRevalidatesIndependentReceipt(t *testing.T) {
	fixture := newStandaloneProvenanceReleaseFixture(t, syntheticRawPerfData())
	if len(fixture.artifacts) == 0 {
		return
	}
	artifact := fixture.artifacts[0]
	authority, ok := fixture.ledger.standaloneSourceReceiptForArtifactPath(artifact.Path)
	if !ok {
		t.Fatal("legal standalone artifact has no ledger receipt")
	}
	index, ok := fixture.ledger.byPath[authority.segment.BindingPath]
	if !ok || fixture.ledger.created[index].standaloneSource == nil {
		t.Fatal("legal standalone receipt is not attached to a created generation")
	}
	fixture.ledger.created[index].standaloneSource.segment.BindingPath = "relative-forged-binding"
	if err := fixture.ledger.validateOwnedPaths(); err == nil {
		t.Fatal("final commit validation accepted a mutated standalone ledger receipt")
	}
}

func TestStandaloneRawProvenanceRejectsEveryReceiptCriticalPublicMutation(t *testing.T) {
	fixture := newStandaloneProvenanceReleaseFixture(t, syntheticRawPerfData())
	if len(fixture.artifacts) == 0 {
		return
	}
	valid := fixture.artifacts[0]
	authority := standaloneReleaseAuthoritySegment(t, fixture, valid)
	measuredBytes, measuredSHA := measuredStandaloneReleaseArtifact(t, valid)
	if err := validateStandaloneRawArtifactClaim(valid, authority, measuredBytes, measuredSHA); err != nil {
		t.Fatalf("legal extracted standalone artifact failed its publication claim: %v", err)
	}

	mutations := []struct {
		name   string
		mutate func(*Artifact)
	}{
		{name: "type", mutate: func(item *Artifact) { item.Type = ArtifactPerfTrace }},
		{name: "type_whitespace", mutate: func(item *Artifact) { item.Type = " " + item.Type }},
		{name: "path", mutate: func(item *Artifact) { item.Path += ".relabelled" }},
		{name: "path_empty", mutate: func(item *Artifact) { item.Path = "" }},
		{name: "path_whitespace", mutate: func(item *Artifact) { item.Path = " " + item.Path }},
		{name: "path_clean_alias", mutate: func(item *Artifact) {
			item.Path = filepath.Dir(item.Path) + string(os.PathSeparator) + "alias" +
				string(os.PathSeparator) + ".." + string(os.PathSeparator) + filepath.Base(item.Path)
		}},
		{name: "bytes", mutate: func(item *Artifact) { item.Bytes++ }},
		{name: "sha256", mutate: func(item *Artifact) { item.SHA256 = hex.EncodeToString(make([]byte, sha256.Size)) }},
		{name: "data_type", mutate: func(item *Artifact) { item.DataType++ }},
		{name: "plugin_name", mutate: func(item *Artifact) { item.PluginName += "-forged" }},
		{name: "plugin_version", mutate: func(item *Artifact) { item.PluginVersion += "-forged" }},
		{name: "source_offset", mutate: func(item *Artifact) { item.SourceOffset++ }},
		{name: "source_bytes", mutate: func(item *Artifact) { item.SourceBytes++ }},
		{name: "converter", mutate: func(item *Artifact) { item.Converter += "-forged" }},
		{name: "converter_whitespace", mutate: func(item *Artifact) { item.Converter = " " + item.Converter }},
		{name: "unexpected_trace_capability", mutate: func(item *Artifact) { item.Trace = &TraceArtifactCapability{} }},
		{name: "standalone_absent", mutate: func(item *Artifact) { item.Standalone = nil }},
		{name: "standalone_profile", mutate: func(item *Artifact) { item.Standalone.Profile += "-forged" }},
		{name: "standalone_layout", mutate: func(item *Artifact) { item.Standalone.LayoutAuthority += "-forged" }},
		{name: "standalone_writer", mutate: func(item *Artifact) { item.Standalone.WriterProfile += "-forged" }},
		{name: "perf_absent", mutate: func(item *Artifact) { item.Perf = nil }},
		{name: "perf_provider_kind", mutate: func(item *Artifact) { item.Perf.ProviderKind += "-forged" }},
		{name: "perf_provider_name", mutate: func(item *Artifact) { item.Perf.ProviderName += "-forged" }},
		{name: "perf_input_format", mutate: func(item *Artifact) { item.Perf.InputFormat += "-forged" }},
		{name: "perf_output_format", mutate: func(item *Artifact) { item.Perf.OutputFormat += "-forged" }},
		{name: "perf_time_domain", mutate: func(item *Artifact) { item.Perf.TimeDomain += "-forged" }},
		{name: "perf_time_alignment", mutate: func(item *Artifact) { item.Perf.TimeAlignment += "-forged" }},
		{name: "perf_thread_identity", mutate: func(item *Artifact) { item.Perf.ThreadIdentity += "-forged" }},
		{name: "perf_cpu_identity", mutate: func(item *Artifact) { item.Perf.CPUIdentity += "-forged" }},
		{name: "perf_event_weight", mutate: func(item *Artifact) { item.Perf.EventWeight += "-forged" }},
		{name: "perf_symbolization", mutate: func(item *Artifact) { item.Perf.Symbolization += "-forged" }},
		{name: "perf_callchain", mutate: func(item *Artifact) { item.Perf.Callchain += "-forged" }},
		{name: "perf_dso_label", mutate: func(item *Artifact) { item.Perf.DSOLabel += "-forged" }},
		{name: "perf_build_id", mutate: func(item *Artifact) { item.Perf.BuildID += "-forged" }},
		{name: "perf_off_cpu", mutate: func(item *Artifact) { item.Perf.OffCPU += "-forged" }},
		{name: "perf_confidence", mutate: func(item *Artifact) { item.Perf.Confidence += "-forged" }},
		{name: "perf_trace_query_ready", mutate: func(item *Artifact) { item.Perf.TraceQueryReady = !item.Perf.TraceQueryReady }},
		{name: "perf_degraded", mutate: func(item *Artifact) { item.Perf.Degraded = !item.Perf.Degraded }},
		{name: "perf_capture_completeness", mutate: func(item *Artifact) { item.Perf.RawCaptureCompleteness = &RawPerfCaptureCompleteness{} }},
		{name: "perf_capture_residual", mutate: func(item *Artifact) { item.Perf.RawCaptureResidual = &RawPerfCaptureResidual{} }},
		{name: "perf_sample_admission", mutate: func(item *Artifact) { item.Perf.RawSampleAdmission = &RawPerfSampleAdmission{} }},
		{name: "perf_caveats", mutate: func(item *Artifact) { item.Perf.Caveats = append(item.Perf.Caveats, "forged") }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			mutated := cloneArtifact(valid)
			test.mutate(&mutated)
			for _, order := range [][]Artifact{{valid, mutated}, {mutated, valid}} {
				if normalized := dedupeArtifacts(order); len(normalized) != 2 {
					t.Fatalf("dedupe swallowed public mutation %q in input order %+v", test.name, order)
				}
			}
			if err := validateStandaloneRawArtifactClaim(mutated, authority, measuredBytes, measuredSHA); err == nil {
				t.Fatalf("receipt-critical public mutation %q was accepted: %+v", test.name, mutated)
			}
		})
	}
}

func TestStandaloneSamePublicPathConflictingDuplicateFailsBothOrders(t *testing.T) {
	fixture := newStandaloneProvenanceReleaseFixture(t, syntheticRawPerfData())
	if len(fixture.artifacts) == 0 {
		return
	}
	valid := cloneArtifact(fixture.artifacts[0])
	forged := cloneArtifact(valid)
	forged.Converter += "-forged"
	for _, test := range []struct {
		name      string
		artifacts []Artifact
	}{
		{name: "valid_then_forged", artifacts: []Artifact{valid, forged}},
		{name: "forged_then_valid", artifacts: []Artifact{forged, valid}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if normalized := dedupeArtifacts(test.artifacts); len(normalized) != 2 {
				t.Fatalf("dedupe swallowed same-path conflicting standalone claim: %+v", normalized)
			}
			if _, err := writeTraceBundleWithAllCoverageAndLedger(
				context.Background(), fixture.inputPath, filepath.Join(t.TempDir(), "same-path.systrace"),
				test.artifacts, nil, nil, nil, nil, nil, fixture.ledger,
			); err == nil {
				t.Fatal("tracebundle publication accepted same-path conflicting standalone claims")
			}
		})
	}
}

func TestStandaloneReceiptBackedEmptyPublicIdentityFailsLoud(t *testing.T) {
	fixture := newStandaloneProvenanceReleaseFixture(t, syntheticRawPerfData())
	if len(fixture.artifacts) == 0 {
		return
	}
	for _, test := range []struct {
		name   string
		mutate func(*Artifact)
	}{
		{name: "empty_path", mutate: func(item *Artifact) { item.Path = "" }},
		{name: "whitespace_path", mutate: func(item *Artifact) { item.Path = "   " }},
		{name: "empty_type_and_path", mutate: func(item *Artifact) {
			item.Type = ""
			item.Path = ""
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutated := cloneArtifact(fixture.artifacts[0])
			test.mutate(&mutated)
			normalized := dedupeArtifacts([]Artifact{mutated})
			if len(normalized) != 1 || normalized[0].standaloneReceipt == nil || normalized[0].Standalone == nil {
				t.Fatalf("dedupe silently discarded receipt-backed empty identity: %+v", normalized)
			}
			if _, err := writeTraceBundleWithAllCoverageAndLedger(
				context.Background(), fixture.inputPath, filepath.Join(t.TempDir(), "empty-identity.systrace"),
				[]Artifact{mutated}, nil, nil, nil, nil, nil, fixture.ledger,
			); err == nil {
				t.Fatal("tracebundle silently accepted or discarded a receipt-backed empty public identity")
			}
		})
	}
}

func TestStandaloneRawProvenanceRejectsPrivateReceiptMutation(t *testing.T) {
	fixture := newStandaloneProvenanceReleaseFixture(t, syntheticRawPerfData())
	if len(fixture.artifacts) == 0 {
		return
	}
	valid := fixture.artifacts[0]
	authority := standaloneReleaseAuthoritySegment(t, fixture, valid)
	measuredBytes, measuredSHA := measuredStandaloneReleaseArtifact(t, valid)
	mutations := []struct {
		name   string
		mutate func(*Artifact)
	}{
		{name: "receipt_absent", mutate: func(item *Artifact) { item.standaloneReceipt = nil }},
		{name: "offset", mutate: func(item *Artifact) { item.standaloneReceipt.Offset++ }},
		{name: "length", mutate: func(item *Artifact) { item.standaloneReceipt.Length++ }},
		{name: "data_type", mutate: func(item *Artifact) { item.standaloneReceipt.DataType++ }},
		{name: "plugin_name", mutate: func(item *Artifact) { item.standaloneReceipt.PluginName += "-forged" }},
		{name: "plugin_version", mutate: func(item *Artifact) { item.standaloneReceipt.PluginVersion += "-forged" }},
		{name: "integrity", mutate: func(item *Artifact) { item.standaloneReceipt.Integrity = "forged" }},
		{name: "layout", mutate: func(item *Artifact) { item.standaloneReceipt.Layout = "forged" }},
		{name: "payload_sha", mutate: func(item *Artifact) { item.standaloneReceipt.PayloadSHA256[0] ^= 0xff }},
		{name: "perf_eligible", mutate: func(item *Artifact) { item.standaloneReceipt.PerfEligible = false }},
		{name: "perf_input_format", mutate: func(item *Artifact) { item.standaloneReceipt.PerfInputFormat = perfInputUnknown }},
		{name: "artifact_path", mutate: func(item *Artifact) { item.standaloneReceipt.ArtifactPath += ".relabelled" }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			mutated := cloneArtifact(valid)
			test.mutate(&mutated)
			if err := validateStandaloneRawArtifactClaim(mutated, authority, measuredBytes, measuredSHA); err == nil {
				t.Fatalf("private receipt mutation %q was accepted", test.name)
			}
		})
	}
}

func TestStandaloneRawProvenanceBundleRejectsReceiptBindingRelabel(t *testing.T) {
	fixture := newStandaloneProvenanceReleaseFixture(t, syntheticRawPerfData())
	if len(fixture.artifacts) == 0 {
		return
	}
	mutated := cloneArtifact(fixture.artifacts[0])
	authority := standaloneReleaseAuthoritySegment(t, fixture, mutated)
	mutated.standaloneReceipt.BindingPath += ".relabelled"
	measuredBytes, measuredSHA := measuredStandaloneReleaseArtifact(t, mutated)
	if err := validateStandaloneRawArtifactClaim(mutated, authority, measuredBytes, measuredSHA); err == nil {
		t.Fatal("direct validator accepted a relabelled standalone receipt binding path")
	}
	if _, err := writeTraceBundleWithAllCoverageAndLedger(
		context.Background(), fixture.inputPath, filepath.Join(t.TempDir(), "binding-relabel.systrace"),
		[]Artifact{mutated}, nil, nil, nil, nil, nil, fixture.ledger,
	); err == nil {
		t.Fatal("tracebundle publication accepted a relabelled standalone receipt binding path")
	}
}

func TestStandaloneDirectExactCannotBeRelabelledAsOfficialZeroWriter(t *testing.T) {
	fixture := newStandaloneProvenanceReleaseFixture(t, syntheticRawPerfData())
	if len(fixture.artifacts) == 0 {
		return
	}
	mutated := cloneArtifact(fixture.artifacts[0])
	authority := standaloneReleaseAuthoritySegment(t, fixture, mutated)
	if mutated.standaloneReceipt.Layout != standaloneLayoutDirectOffsetZero ||
		mutated.standaloneReceipt.Integrity != standaloneIntegrityPayloadSHA256 {
		t.Fatalf("fixture is not a direct exact-SHA source: %+v", mutated.standaloneReceipt)
	}
	mutated.standaloneReceipt.Integrity = standaloneIntegrityOfficialZero
	mutated.Standalone.WriterProfile = standaloneIntegrityOfficialZero
	measuredBytes, measuredSHA := measuredStandaloneReleaseArtifact(t, mutated)
	if err := validateStandaloneRawArtifactClaim(mutated, authority, measuredBytes, measuredSHA); err == nil {
		t.Fatal("direct exact-SHA source was accepted after a coherent public/private official-zero relabel")
	}
	if _, err := writeTraceBundleWithAllCoverageAndLedger(
		context.Background(), fixture.inputPath, filepath.Join(t.TempDir(), "official-zero-relabel.systrace"),
		[]Artifact{mutated}, nil, nil, nil, nil, nil, fixture.ledger,
	); err == nil {
		t.Fatal("tracebundle publication accepted a direct source relabelled as an official-zero writer")
	}
}

func TestStandaloneLedgerAuthorityRejectsCoherentArtifactReceiptRelabels(t *testing.T) {
	fixture := newStandaloneProvenanceReleaseFixture(t, syntheticRawPerfData())
	if len(fixture.artifacts) == 0 {
		return
	}
	valid := fixture.artifacts[0]
	authority := standaloneReleaseAuthoritySegment(t, fixture, valid)
	measuredBytes, measuredSHA := measuredStandaloneReleaseArtifact(t, valid)
	for _, test := range []struct {
		name   string
		mutate func(*Artifact)
	}{
		{name: "public_and_private_offset", mutate: func(item *Artifact) {
			item.SourceOffset++
			item.standaloneReceipt.Offset++
		}},
		{name: "public_and_private_root_profile_layout", mutate: func(item *Artifact) {
			item.Standalone.LayoutAuthority = standaloneLayoutRootProfile
			item.standaloneReceipt.Layout = standaloneLayoutRootProfile
		}},
		{name: "root_profile_official_zero_public_private", mutate: func(item *Artifact) {
			item.Standalone.LayoutAuthority = standaloneLayoutRootProfile
			item.standaloneReceipt.Layout = standaloneLayoutRootProfile
			item.Standalone.WriterProfile = standaloneIntegrityOfficialZero
			item.standaloneReceipt.Integrity = standaloneIntegrityOfficialZero
		}},
		{name: "relative_binding_alias", mutate: func(item *Artifact) {
			item.standaloneReceipt.BindingPath = "relative" + string(os.PathSeparator) + ".." +
				string(os.PathSeparator) + filepath.Base(item.standaloneReceipt.BindingPath)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutated := cloneArtifact(valid)
			test.mutate(&mutated)
			if err := validateStandaloneRawArtifactClaim(
				mutated, authority, measuredBytes, measuredSHA,
			); err == nil {
				t.Fatalf("direct validator accepted coherent artifact/receipt relabel %q", test.name)
			}
			if _, err := writeTraceBundleWithAllCoverageAndLedger(
				context.Background(), fixture.inputPath, filepath.Join(t.TempDir(), "coherent-relabel.systrace"),
				[]Artifact{mutated}, nil, nil, nil, nil, nil, fixture.ledger,
			); err == nil {
				t.Fatalf("tracebundle publication accepted coherent artifact/receipt relabel %q", test.name)
			}
		})
	}
}

func TestStandaloneIdenticalPayloadPathSwapSurvivesDedupeAndFailsBothOrders(t *testing.T) {
	payload := syntheticRawPerfData()
	fixture := newStandaloneProvenanceReleaseFixture(t, payload, append([]byte(nil), payload...))
	if len(fixture.artifacts) == 0 {
		return
	}
	if len(fixture.artifacts) != 2 {
		t.Fatalf("raw artifact count=%d want=2", len(fixture.artifacts))
	}
	first := cloneArtifact(fixture.artifacts[0])
	second := cloneArtifact(fixture.artifacts[1])
	firstBody, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	secondBody, err := os.ReadFile(second.Path)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 || first.Bytes != second.Bytes ||
		!bytes.Equal(firstBody, secondBody) {
		t.Fatalf("fixture payloads are not byte-identical: first=%+v second=%+v", first, second)
	}
	first.Path, second.Path = second.Path, first.Path

	for _, test := range []struct {
		name      string
		artifacts []Artifact
	}{
		{name: "source_order", artifacts: []Artifact{first, second}},
		{name: "reverse_order", artifacts: []Artifact{second, first}},
	} {
		t.Run(test.name, func(t *testing.T) {
			normalized := dedupeArtifacts(test.artifacts)
			if len(normalized) != 2 {
				t.Fatalf("dedupe swallowed a forged identical-payload claim: got=%d artifacts=%+v", len(normalized), normalized)
			}
			bundleAnchor := filepath.Join(t.TempDir(), "path-swap.systrace")
			if _, err := writeTraceBundleWithAllCoverageAndLedger(
				context.Background(), fixture.inputPath, bundleAnchor,
				test.artifacts, nil, nil, nil, nil, nil, fixture.ledger,
			); err == nil {
				t.Fatal("tracebundle publication accepted swapped standalone source paths")
			}
		})
	}
}
