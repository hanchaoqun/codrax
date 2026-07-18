package hitraceconv

import (
	"context"
	"encoding/hex"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeOneValidatedPerfTraceForClaimTest(
	t *testing.T,
	profile ownedTracePerfProfile,
	path string,
	ledger *conversionFileLedger,
) {
	t.Helper()
	ctx := context.Background()
	var err error
	switch profile {
	case ownedTracePerfSimpleperfText:
		err = writeSimpleperfSamplesToPerfTraceWithLedger(ctx, []simpleperfSample{{
			Comm: "app", PID: 10, TID: 11, CPU: 1, TimestampNS: 1_000_000_000, Period: 7, Event: "cycles",
			Leaf: simpleperfFrame{IP: "0x10", Symbol: "Hot", DSO: "lib.so"},
		}}, path, ledger)
	case ownedTracePerfSimpleperfProto:
		err = writeSimpleperfProtoDataToPerfTraceWithLedger(ctx, simpleperfProtoData{
			Files: map[uint32]simpleperfProtoFile{}, Threads: map[uint32]simpleperfProtoThread{11: {TID: 11, PID: 10, Name: "app"}},
			Samples: []simpleperfProtoSample{{TimeNS: 1_000_000_000, ThreadID: 11, EventCount: 7}},
		}, path, ledger)
	case ownedTracePerfHiperfProto:
		err = writeHiperfProtoDataToPerfTraceWithLedger(ctx, hiperfProtoData{
			Files: map[uint32]hiperfProtoFile{}, Threads: map[uint32]hiperfProtoThread{11: {TID: 11, PID: 10, Name: "app"}},
			Samples: []hiperfProtoSample{{TimeNS: 1_000_000_000, TID: 11, EventCount: 7}},
		}, path, ledger)
	case ownedTracePerfRaw:
		capture := newRawPerfCaptureCompleteness()
		capture.SampleRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
		admission := rawPerfTestQueryableAdmission(1)
		err = finishRawPerfDataConversion(ctx, "input.perf.data", path, nil, ledger, time.Time{}, rawPerfData{
			Samples:             []rawPerfSample{{PID: 10, TID: 11, CPU: 1, CPUValid: true, TimeNS: 1_000_000_000, IP: 0x10, Period: 7}},
			CaptureCompleteness: capture,
			SampleAdmission:     admission,
		}, nil)
	default:
		t.Fatalf("unsupported claim fixture profile %q", profile)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidatedPerfArtifactAndDecisionConsumeExactReceipt(t *testing.T) {
	tests := []struct {
		profile      ownedTracePerfProfile
		providerName string
		providerKind string
		inputFormat  perfInputFormat
	}{
		{ownedTracePerfSimpleperfText, perfProviderNameSimpleperfText, perfProviderKindOfficialAndroid, perfInputLinuxPerfData},
		{ownedTracePerfSimpleperfProto, perfProviderNameSimpleperfProto, perfProviderKindOfficialAndroid, perfInputSimpleperfReportProto},
		{ownedTracePerfHiperfProto, perfProviderNameHiperfProto, perfProviderKindOfficialHarmony, perfInputLinuxPerfData},
		{ownedTracePerfRaw, perfProviderNameRawFallback, perfProviderKindRawFallback, perfInputLinuxPerfData},
	}
	for _, test := range tests {
		t.Run(string(test.profile), func(t *testing.T) {
			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.cleanup()
			path := filepath.Join(t.TempDir(), string(test.profile)+".perftrace")
			writeOneValidatedPerfTraceForClaimTest(t, test.profile, path, ledger)

			published, ok := ledger.ownedTraceValidation(path)
			if !ok {
				t.Fatal("validated public receipt is absent")
			}
			artifact, err := newValidatedPerfTraceArtifact(ledger, path, test.profile, test.inputFormat, "fixture", []string{"kept"})
			if err != nil {
				t.Fatal(err)
			}
			spec, _ := test.profile.claimSpec()
			if artifact.Type != ArtifactPerfTrace || artifact.Path != path || artifact.Bytes != published.receipt.size ||
				artifact.SHA256 != hex.EncodeToString(published.receipt.wireSHA256[:]) || artifact.Converter != spec.converter ||
				artifact.Perf == nil || !artifact.Perf.TraceQueryReady || artifact.Perf.ProviderName != test.providerName ||
				artifact.Perf.ProviderKind != test.providerKind || len(artifact.Caveats) != 1 || artifact.Caveats[0] != "kept" {
				t.Fatalf("artifact did not project exact receipt/profile: %+v receipt=%+v", artifact, published.receipt)
			}

			stage := perfProviderStageDirectInput
			if test.profile == ownedTracePerfHiperfProto {
				stage = perfProviderStageStandaloneHiperf
			}
			opts := Options{}
			if test.profile == ownedTracePerfRaw {
				opts.PerfParser = "raw"
			}
			decision := newPerfProviderDecision(stage, perfProviderByName(test.providerName), opts, "input", test.inputFormat, path)
			if test.profile == ownedTracePerfRaw {
				decision.Fallback = false
			}
			decision, err = perfProviderSuccess(decision, artifact, ledger)
			if err != nil || !decision.Selected || !decision.Attempted || !decision.Succeeded ||
				!decision.TraceQueryReady || decision.ArtifactPath != path {
				t.Fatalf("decision did not project exact receipt: %+v err=%v", decision, err)
			}
			forgedProvider := newPerfProviderDecision(stage, perfProviderByName(test.providerName), opts, "input", test.inputFormat, path)
			if test.profile == ownedTracePerfRaw {
				forgedProvider.Fallback = false
			}
			forgedProvider.ProviderName = " " + test.providerName
			if _, err := perfProviderSuccess(forgedProvider, artifact, ledger); !ownedTraceOutputHardFailure(err) {
				t.Fatalf("non-canonical provider name escaped closed mapping: %v", err)
			}

			forged := artifact
			forged.Bytes++
			forgedDecision := newPerfProviderDecision(stage, perfProviderByName(test.providerName), opts, "input", test.inputFormat, path)
			if test.profile == ownedTracePerfRaw {
				forgedDecision.Fallback = false
			}
			if _, err := perfProviderSuccess(forgedDecision, forged, ledger); !ownedTraceOutputHardFailure(err) {
				t.Fatalf("forged artifact escaped hard decision gate: %v", err)
			}
			semanticDrifts := []struct {
				name   string
				mutate func(*PerfArtifactCapability)
			}{
				{"time_domain", func(capability *PerfArtifactCapability) { capability.TimeDomain = "forged_clock" }},
				{"cpu_identity", func(capability *PerfArtifactCapability) { capability.CPUIdentity = "forged_cpu_identity" }},
				{"thread_identity", func(capability *PerfArtifactCapability) { capability.ThreadIdentity = "tid_only" }},
				{"event_weight", func(capability *PerfArtifactCapability) { capability.EventWeight = "unweighted" }},
				{"degraded", func(capability *PerfArtifactCapability) { capability.Degraded = !capability.Degraded }},
				{"fixed_caveat", func(capability *PerfArtifactCapability) {
					if len(capability.Caveats) == 0 {
						capability.Caveats = []string{"forged limitation"}
						return
					}
					capability.Caveats = append([]string(nil), capability.Caveats...)
					capability.Caveats[0] = "forged limitation"
				}},
			}
			for _, drift := range semanticDrifts {
				t.Run("reject_"+drift.name, func(t *testing.T) {
					forged := artifact
					capability := *artifact.Perf
					forged.Perf = &capability
					drift.mutate(forged.Perf)
					decision := newPerfProviderDecision(stage, perfProviderByName(test.providerName), opts, "input", test.inputFormat, path)
					if test.profile == ownedTracePerfRaw {
						decision.Fallback = false
					}
					if _, err := perfProviderSuccess(decision, forged, ledger); !ownedTraceOutputHardFailure(err) {
						t.Fatalf("semantic drift escaped hard decision gate: %+v err=%v", forged.Perf, err)
					}
				})
			}
			if test.profile == ownedTracePerfRaw {
				forged := cloneArtifact(artifact)
				forged.Perf.RawSampleAdmission.QueryRows++
				decision := newPerfProviderDecision(stage, perfProviderByName(test.providerName), opts, "input", test.inputFormat, path)
				decision.Fallback = false
				if _, err := perfProviderSuccess(decision, forged, ledger); !ownedTraceOutputHardFailure(err) {
					t.Fatalf("sample-admission drift escaped exact receipt gate: %+v err=%v", forged.Perf.RawSampleAdmission, err)
				}
			}
		})
	}
}

func TestValidatedPerfClaimFailsClosedWithoutExactProfileReceipt(t *testing.T) {
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	path := filepath.Join(t.TempDir(), "claimless.perftrace")
	if err := os.WriteFile(path, []byte("claimless"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		path    string
		profile ownedTracePerfProfile
	}{
		{"missing_receipt", path, ownedTracePerfSimpleperfText},
		{"open_profile", path, ownedTracePerfProfile("simpleperf")},
		{"empty_path", "", ownedTracePerfSimpleperfText},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := validatedOwnedPerfTraceClaim(ledger, test.path, test.profile)
			var publication *ownedTracePublicationError
			if !errors.As(err, &publication) || publication == nil || !ownedTraceOutputHardFailure(err) {
				t.Fatalf("claim failure lost hard typed identity: %T %v", err, err)
			}
		})
	}

	validatedPath := filepath.Join(t.TempDir(), "validated.perftrace")
	writeOneValidatedPerfTraceForClaimTest(t, ownedTracePerfSimpleperfText, validatedPath, ledger)
	if _, err := newValidatedPerfTraceArtifact(ledger, validatedPath, ownedTracePerfRaw, perfInputLinuxPerfData, "fixture", nil); !ownedTraceOutputHardFailure(err) {
		t.Fatalf("cross-profile relabel escaped: %v", err)
	}
	if _, err := newValidatedPerfTraceArtifact(ledger, validatedPath, ownedTracePerfSimpleperfText, perfInputGzipPerfData, "fixture", nil); !ownedTraceOutputHardFailure(err) {
		t.Fatalf("unsupported input format escaped closed provider profile: %v", err)
	}
}

func TestPerfReadyMintStructureIsReceiptOnly(t *testing.T) {
	for _, capability := range []*PerfArtifactCapability{
		perfCapabilityForSimpleperfReportSample(perfInputLinuxPerfData, "test"),
		perfCapabilityForSimpleperfReportProto("test"),
		perfCapabilityForHiperfProto(perfInputLinuxPerfData, "test"),
		perfCapabilityForRawFallback(perfInputLinuxPerfData),
	} {
		if capability == nil || capability.TraceQueryReady {
			t.Fatalf("base capability minted readiness without a receipt: %+v", capability)
		}
	}
	providerBody := sourceGenerationFunctionBody(t, "perf_provider.go", "perfProviderSuccess")
	for _, forbidden := range []string{
		"artifact.Type == ArtifactPerfTrace",
		"decision.TraceQueryReady = artifact.Perf.TraceQueryReady",
		"decision.TraceQueryReady = artifact.Type",
	} {
		if strings.Contains(providerBody, forbidden) {
			t.Fatalf("provider success still mints readiness from %q:\n%s", forbidden, providerBody)
		}
	}
	if !strings.Contains(providerBody, "validateOwnedPerfTraceArtifactClaim(") ||
		!strings.Contains(providerBody, "decision.TraceQueryReady = published.receipt.queryReady") {
		t.Fatalf("provider success does not consume exact receipt:\n%s", providerBody)
	}

	providers := []struct{ file, function string }{
		{"simpleperf_text.go", "maybeConvertSimpleperfPerfData"},
		{"simpleperf_proto.go", "maybeConvertSimpleperfProtoWithDecision"},
		{"simpleperf_proto.go", "maybeConvertSimpleperfProtoFromInputWithDecision"},
		{"hiperf_proto.go", "maybeConvertHiperfPerfDataFromInput"},
		{"raw_perfdata.go", "maybeConvertRawPerfData"},
		{"raw_perfdata.go", "maybeConvertRawPerfDataFromInput"},
		{"raw_perfdata.go", "maybeConvertRawPerfDataFromStandaloneInput"},
	}
	for _, provider := range providers {
		body := sourceGenerationFunctionBody(t, provider.file, provider.function)
		if strings.Contains(body, "os.Lstat(perfTracePath)") || !strings.Contains(body, "newValidatedPerfTraceArtifact(") {
			t.Fatalf("%s still mints Artifact from a public path/type:\n%s", provider.function, body)
		}
	}

	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve perf claim structure test path")
	}
	dir := filepath.Dir(current)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	artifactMints := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), name, body, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			if assignment, ok := node.(*ast.AssignStmt); ok {
				for index, left := range assignment.Lhs {
					if index >= len(assignment.Rhs) {
						break
					}
					selector, selectorOK := left.(*ast.SelectorExpr)
					value, valueOK := assignment.Rhs[index].(*ast.Ident)
					if !selectorOK || !valueOK {
						continue
					}
					if selector.Sel.Name == "Type" && value.Name == "ArtifactPerfTrace" {
						t.Fatalf("production file %s minted ArtifactPerfTrace by assignment", name)
					}
					if selector.Sel.Name == "TraceQueryReady" && value.Name == "true" {
						t.Fatalf("production file %s minted trace-query readiness from a boolean literal", name)
					}
				}
			}
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			literalType, ok := literal.Type.(*ast.Ident)
			if !ok {
				return true
			}
			if literalType.Name == "PerfArtifactCapability" {
				for _, element := range literal.Elts {
					field, ok := element.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, keyOK := field.Key.(*ast.Ident)
					value, valueOK := field.Value.(*ast.Ident)
					if keyOK && valueOK && key.Name == "TraceQueryReady" && value.Name == "true" {
						t.Fatalf("production file %s minted perf readiness in a composite literal", name)
					}
				}
				return true
			}
			if literalType.Name != "Artifact" {
				return true
			}
			if len(literal.Elts) > 0 {
				if value, ok := literal.Elts[0].(*ast.Ident); ok && value.Name == "ArtifactPerfTrace" {
					artifactMints++
					if name != "owned_perftrace_claim.go" {
						t.Fatalf("production file %s minted ArtifactPerfTrace in an unkeyed literal", name)
					}
				}
			}
			for _, element := range literal.Elts {
				field, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, keyOK := field.Key.(*ast.Ident)
				value, valueOK := field.Value.(*ast.Ident)
				if keyOK && valueOK && key.Name == "Type" && value.Name == "ArtifactPerfTrace" {
					artifactMints++
					if name != "owned_perftrace_claim.go" {
						t.Fatalf("production file %s minted ArtifactPerfTrace outside the receipt factory", name)
					}
				}
			}
			return true
		})
		if name != "owned_perftrace_claim.go" && strings.Contains(string(body), "capability.TraceQueryReady =") {
			t.Fatalf("production file %s minted perf capability readiness outside the receipt factory", name)
		}
	}
	if artifactMints != 1 {
		t.Fatalf("owned perftrace Artifact mint count=%d, want exactly one", artifactMints)
	}
}
