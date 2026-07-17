package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func profilerRootProfileMessage(index int) []byte {
	line := fmt.Sprintf("worker-%d (%d) [001] .... %d.000000: print: B|%d|RootProfile%d",
		index, index, index, index, index)
	return syntheticProfilerPluginData("bytrace_plugin", []byte(line))
}

func profilerRootSequentialFixture(messages ...[]byte) []byte {
	return syntheticProfilerTraceFile(messages...)
}

func profilerRootProfileRandomWriter(messages ...[]byte) []byte {
	body := syntheticProfilerTraceFile(messages...)
	binary.LittleEndian.PutUint32(body[20:24], uint32(len(messages)))
	empty := sha256.Sum256(nil)
	copy(body[24:56], empty[:])
	return body
}

func profilerRootProfileSealSequential(body []byte, segments uint32) {
	binary.LittleEndian.PutUint32(body[20:24], segments)
	digest := sha256.Sum256(body[profilerTraceHeaderSize:])
	copy(body[24:56], digest[:])
}

func extractProfilerRootProfileFixture(t testing.TB, body []byte) (profilerContainerExtraction, *traceDBRowSink) {
	t.Helper()
	header, ok := readProfilerTraceHeaderAt(bytes.NewReader(body), 0, int64(len(body)))
	if !ok {
		t.Fatal("read root TraceFile header")
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	extracted, err := extractProfilerTraceFileAtWithFrameLimit(
		context.Background(), bytes.NewReader(body), int64(len(body)), header, sink, maxProfilerPluginFrameBytes)
	if err != nil {
		_ = sink.cleanup()
		t.Fatalf("extract root TraceFile: %v", err)
	}
	return extracted, sink
}

func profilerRootProfileIntegrityBarrier(extracted profilerContainerExtraction) bool {
	for _, item := range extracted.TraceCoverage {
		if item.Table == "__container_integrity_barrier__" &&
			item.FieldSources["failure_class"] == "integrity_failure" {
			return true
		}
	}
	return false
}

func profilerRootWriterProfile(extracted profilerContainerExtraction) string {
	for _, caveat := range extracted.Caveats {
		for _, profile := range []string{
			profilerRootWriterProfileEmpty,
			profilerRootWriterProfileSequential,
			profilerRootWriterProfileFDRandom,
		} {
			if strings.Contains(caveat, "writer_profile="+profile) {
				return profile
			}
		}
	}
	return ""
}

func requireProfilerRootProfileFailClosed(t testing.TB, extracted profilerContainerExtraction, sink *traceDBRowSink) {
	t.Helper()
	if !extracted.Detected || extracted.Kind != "openharmony_profiler_trace_file" ||
		!extracted.SourceFailClosed || extracted.SourceFailReason == "" ||
		extracted.TextRows != 0 || extracted.StructuredRows != 0 ||
		!sink.allRowsFailClosed || sink.publishableRows() != 0 ||
		!profilerRootProfileIntegrityBarrier(extracted) {
		t.Fatalf("invalid root profile was not integrity-failed closed: extracted=%+v coverage=%+v sink=%+v",
			extracted, extracted.TraceCoverage, sink.stats)
	}
}

func TestProfilerRootProfileOfficialWriterProfiles(t *testing.T) {
	profiles := []struct {
		name  string
		build func(...[]byte) []byte
	}{
		{name: "sequential", build: profilerRootSequentialFixture},
		{name: "fd_random", build: profilerRootProfileRandomWriter},
	}
	for _, profile := range profiles {
		for _, count := range []int{0, 1, 3} {
			t.Run(fmt.Sprintf("%s_n%d", profile.name, count), func(t *testing.T) {
				messages := make([][]byte, 0, count)
				for index := 1; index <= count; index++ {
					messages = append(messages, profilerRootProfileMessage(index))
				}
				body := profile.build(messages...)
				extracted, sink := extractProfilerRootProfileFixture(t, body)
				defer sink.cleanup()
				if extracted.SourceFailClosed || sink.allRowsFailClosed ||
					extracted.TextRows != count || sink.publishableRows() != count {
					t.Fatalf("official %s writer N=%d rejected: extracted=%+v sink=%+v",
						profile.name, count, extracted, sink.stats)
				}
				wantWriterProfile := profilerRootWriterProfileSequential
				if count == 0 {
					wantWriterProfile = profilerRootWriterProfileEmpty
				} else if profile.name == "fd_random" {
					wantWriterProfile = profilerRootWriterProfileFDRandom
				}
				if got := profilerRootWriterProfile(extracted); got != wantWriterProfile {
					t.Fatalf("official %s writer N=%d profile=%q want=%q coverage=%+v",
						profile.name, count, got, wantWriterProfile, extracted.TraceCoverage)
				}
			})
		}
	}
}

func TestProfilerRootProfileWriterSpecificZeroFrameAuthority(t *testing.T) {
	sequential := profilerRootSequentialFixture([]byte{})
	extracted, sink := extractProfilerRootProfileFixture(t, sequential)
	defer sink.cleanup()
	if extracted.SourceFailClosed || extracted.Messages != 1 || extracted.RejectedMessages != 1 ||
		profilerRootWriterProfile(extracted) != profilerRootWriterProfileSequential {
		t.Fatalf("sequential writer zero frame must retain physical integrity and semantic rejection: %+v", extracted)
	}

	random := append([]byte(nil), sequential...)
	binary.LittleEndian.PutUint32(random[20:24], 1)
	empty := sha256.Sum256(nil)
	copy(random[24:56], empty[:])
	rejected, rejectedSink := extractProfilerRootProfileFixture(t, random)
	defer rejectedSink.cleanup()
	requireProfilerRootProfileFailClosed(t, rejected, rejectedSink)
	if rejected.SourceFailReason != "profiler_root_fd_random_zero_frame_forbidden" {
		t.Fatalf("fd-random zero frame used an unexpected integrity reason: %+v", rejected)
	}
}

func TestProfilerOffsetZeroHeaderCannotEscapeThroughSessionJSON(t *testing.T) {
	for _, test := range []struct {
		name           string
		dataType       uint32
		wantKind       string
		wantFailReason string
	}{
		{name: "hiperf", dataType: profilerDataTypeHiperf},
		{name: "standalone", dataType: profilerDataTypeStandalone},
		{name: "unknown", dataType: 77, wantKind: "openharmony_profiler_trace_file", wantFailReason: "profiler_root_data_type_unsupported"},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := []byte(profilerSessionJSONTag + "\nworker-1 (1) [000] .... 1.000000: print: B|1|must-not-parse\n")
			body := syntheticStandaloneProfilerBlock(test.dataType, "route-probe", "1.0", payload)
			input := newScriptedStandaloneInputView(filepath.Join(t.TempDir(), test.name+".htrace"), body)
			binding, err := newProfilerInputBinding(input, input.DisplayPath())
			if err != nil {
				t.Fatal(err)
			}
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			extracted, err := extractProfilerContainerSystraceRowsWithSessionLimitFromInput(
				context.Background(), binding, int64(len(body)), sink)
			if err != nil {
				t.Fatal(err)
			}
			if extracted.Kind != test.wantKind || extracted.TextRows != 0 || sink.publishableRows() != 0 ||
				extracted.SourceFailReason != test.wantFailReason {
				t.Fatalf("offset-zero data_type=%d escaped its typed route: extracted=%+v sink=%+v",
					test.dataType, extracted, sink.stats)
			}
		})
	}
}

func TestProfilerRootProfileAuthorityHasSingleProductionOwner(t *testing.T) {
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, ".", func(info os.FileInfo) bool {
		return strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := packages["hitraceconv"]
	if pkg == nil {
		t.Fatal("production hitraceconv package missing")
	}
	targetFunctions := map[string]*ast.FuncDecl{}
	definitionCounts := map[string]int{}
	callOwners := map[string][]string{}
	for filename, file := range pkg.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			owner := function.Name.Name
			if function.Recv != nil && len(function.Recv.List) == 1 {
				receiver := function.Recv.List[0].Type
				if pointer, ok := receiver.(*ast.StarExpr); ok {
					receiver = pointer.X
				}
				if ident, ok := receiver.(*ast.Ident); ok {
					owner = ident.Name + "." + function.Name.Name
				}
			}
			switch owner {
			case "profilerRootHeaderFailure", "newProfilerRootIntegrityLedger", "profilerRootIntegrityLedger.validate":
				definitionCounts[owner]++
			}
			if function.Body == nil {
				continue
			}
			if function.Name.Name == "extractProfilerTraceFileAtWithFrameLimit" ||
				function.Name.Name == "extractProfilerContainerSystraceRowsWithSessionLimitFromInput" {
				targetFunctions[function.Name.Name] = function
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee := ""
				switch typed := call.Fun.(type) {
				case *ast.Ident:
					if typed.Name == "profilerRootHeaderFailure" || typed.Name == "newProfilerRootIntegrityLedger" {
						callee = typed.Name
					}
				case *ast.SelectorExpr:
					if ident, ok := typed.X.(*ast.Ident); ok && ident.Name == "integrity" && typed.Sel.Name == "validate" {
						callee = "profilerRootIntegrityLedger.validate"
					}
				}
				if callee != "" {
					callOwners[callee] = append(callOwners[callee], filepath.Base(filename)+":"+owner)
				}
				return true
			})
		}
	}
	wantOwner := "profiler_container.go:extractProfilerTraceFileAtWithFrameLimit"
	for _, authority := range []string{
		"profilerRootHeaderFailure",
		"newProfilerRootIntegrityLedger",
		"profilerRootIntegrityLedger.validate",
	} {
		if definitionCounts[authority] != 1 || len(callOwners[authority]) != 1 || callOwners[authority][0] != wantOwner {
			t.Fatalf("root profile authority %s lost single ownership: definitions=%d calls=%v",
				authority, definitionCounts[authority], callOwners[authority])
		}
	}

	router := targetFunctions["extractProfilerContainerSystraceRowsWithSessionLimitFromInput"]
	if router == nil {
		t.Fatal("root/session router missing")
	}
	var routeIf *ast.IfStmt
	for _, statement := range router.Body.List {
		candidate, ok := statement.(*ast.IfStmt)
		if !ok {
			continue
		}
		ident, ok := candidate.Cond.(*ast.Ident)
		if ok && ident.Name == "ok" {
			routeIf = candidate
			break
		}
	}
	if routeIf == nil || routeIf.Else != nil || len(routeIf.Body.List) != 2 {
		t.Fatalf("offset-zero official-header route lost closed two-arm shape: %#v", routeIf)
	}
	standaloneIf, standaloneOK := routeIf.Body.List[0].(*ast.IfStmt)
	rootReturn, rootOK := routeIf.Body.List[1].(*ast.ReturnStmt)
	if !standaloneOK || standaloneIf.Else != nil || len(standaloneIf.Body.List) != 1 ||
		!rootOK || len(rootReturn.Results) != 1 {
		t.Fatalf("offset-zero standalone/root route shape drifted: standalone=%#v root=%#v", standaloneIf, rootReturn)
	}
	standaloneCall, standaloneCallOK := standaloneIf.Cond.(*ast.CallExpr)
	if !standaloneCallOK {
		t.Fatalf("known standalone route lost its typed predicate: %#v", standaloneIf.Cond)
	}
	standaloneIdent, standaloneIdentOK := standaloneCall.Fun.(*ast.Ident)
	standaloneReturn, standaloneReturnOK := standaloneIf.Body.List[0].(*ast.ReturnStmt)
	rootCall, rootCallOK := rootReturn.Results[0].(*ast.CallExpr)
	if !rootCallOK {
		t.Fatalf("unknown/root route lost its single extractor call: %#v", rootReturn)
	}
	rootIdent, rootIdentOK := rootCall.Fun.(*ast.Ident)
	if !standaloneCallOK || !standaloneIdentOK || standaloneIdent.Name != "isProfilerStandaloneDataType" ||
		!standaloneReturnOK || len(standaloneReturn.Results) != 2 ||
		!rootCallOK || !rootIdentOK || rootIdent.Name != "extractProfilerTraceFileFromInput" {
		t.Fatalf("known standalone ceased to be the only root bypass: standalone=%#v root=%#v", standaloneIf, rootReturn)
	}
	zeroExtraction, zeroOK := standaloneReturn.Results[0].(*ast.CompositeLit)
	if !zeroOK {
		t.Fatalf("known standalone bypass no longer returns a zero extraction: %#v", standaloneReturn)
	}
	zeroType, zeroTypeOK := zeroExtraction.Type.(*ast.Ident)
	nilResult, nilOK := standaloneReturn.Results[1].(*ast.Ident)
	if !zeroOK || !zeroTypeOK || zeroType.Name != "profilerContainerExtraction" || len(zeroExtraction.Elts) != 0 ||
		!nilOK || nilResult.Name != "nil" {
		t.Fatalf("known standalone bypass must return zero extraction without Session reinterpretation: %#v", standaloneReturn)
	}
}

func TestProfilerRootProfileIntegrityFailuresFailCloseWholeSource(t *testing.T) {
	base := profilerRootSequentialFixture(profilerRootProfileMessage(1))
	tests := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{name: "version_previous", mutate: func(body []byte) []byte {
			binary.LittleEndian.PutUint32(body[16:20], 0x0000ffff)
			return body
		}},
		{name: "version_future_minor", mutate: func(body []byte) []byte {
			binary.LittleEndian.PutUint32(body[16:20], 0x00010001)
			return body
		}},
		{name: "segments_minus_one", mutate: func(body []byte) []byte {
			binary.LittleEndian.PutUint32(body[20:24], 1)
			return body
		}},
		{name: "segments_plus_one", mutate: func(body []byte) []byte {
			binary.LittleEndian.PutUint32(body[20:24], 3)
			return body
		}},
		{name: "segments_max", mutate: func(body []byte) []byte {
			binary.LittleEndian.PutUint32(body[20:24], ^uint32(0))
			return body
		}},
		{name: "sha_single_bit", mutate: func(body []byte) []byte {
			body[24] ^= 0x01
			return body
		}},
		{name: "sha_zero", mutate: func(body []byte) []byte {
			clear(body[24:56])
			return body
		}},
		{name: "sha_wrong_writer_lane", mutate: func(body []byte) []byte {
			empty := sha256.Sum256(nil)
			copy(body[24:56], empty[:])
			return body
		}},
		{name: "declared_header_minus_one", mutate: func(body []byte) []byte {
			binary.LittleEndian.PutUint64(body[8:16], profilerTraceHeaderSize-1)
			return body
		}},
		{name: "declared_body_minus_one", mutate: func(body []byte) []byte {
			binary.LittleEndian.PutUint64(body[8:16], uint64(len(body)-1))
			return body
		}},
		{name: "declared_body_plus_one", mutate: func(body []byte) []byte {
			binary.LittleEndian.PutUint64(body[8:16], uint64(len(body)+1))
			return body
		}},
		{name: "frame_payload_truncated", mutate: func(body []byte) []byte {
			declared := binary.LittleEndian.Uint32(body[profilerTraceHeaderSize : profilerTraceHeaderSize+4])
			binary.LittleEndian.PutUint32(body[profilerTraceHeaderSize:profilerTraceHeaderSize+4], declared+1)
			profilerRootProfileSealSequential(body, 2)
			return body
		}},
	}
	for residual := 1; residual <= 3; residual++ {
		residual := residual
		tests = append(tests, struct {
			name   string
			mutate func([]byte) []byte
		}{name: fmt.Sprintf("truncated_length_prefix_%d", residual), mutate: func(body []byte) []byte {
			body = append(body, bytes.Repeat([]byte{0xa5}, residual)...)
			binary.LittleEndian.PutUint64(body[8:16], uint64(len(body)))
			profilerRootProfileSealSequential(body, 2)
			return body
		}})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := test.mutate(append([]byte(nil), base...))
			extracted, sink := extractProfilerRootProfileFixture(t, body)
			defer sink.cleanup()
			requireProfilerRootProfileFailClosed(t, extracted, sink)
		})
	}
}

func TestProfilerRootProfileUsesTypedTerminalSidecarBoundary(t *testing.T) {
	root := profilerRootSequentialFixture(profilerRootProfileMessage(1))
	sidecar := syntheticStandaloneProfilerBlock(
		profilerDataTypeHiperf, "hiperf-plugin", "1.0", []byte("terminal-perf-payload"))
	full := append(append([]byte(nil), root...), sidecar...)
	namespace := filepath.Join(t.TempDir(), "root-with-terminal-sidecar.htrace")
	input := newScriptedStandaloneInputView(namespace, full)
	binding, err := newProfilerInputBinding(input, namespace)
	if err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	extracted, err := extractProfilerContainerSystraceRowsWithSessionLimitFromInput(
		context.Background(), binding, int64(len(root)), sink)
	if err != nil {
		t.Fatal(err)
	}
	if extracted.SourceFailClosed || extracted.TextRows != 1 || sink.publishableRows() != 1 {
		t.Fatalf("typed terminal sidecar boundary rejected a valid root: extracted=%+v sink=%+v",
			extracted, sink.stats)
	}
}

func TestProfilerRootProfileCannotAbsorbTypedTerminalSidecar(t *testing.T) {
	root := profilerRootSequentialFixture(profilerRootProfileMessage(1))
	sidecar := syntheticStandaloneProfilerBlock(
		profilerDataTypeHiperf, "hiperf-plugin", "1.0", []byte("terminal-perf-payload"))
	full := append(append([]byte(nil), root...), sidecar...)
	binary.LittleEndian.PutUint64(full[8:16], uint64(len(full)))
	profilerRootProfileSealSequential(full, 2)
	namespace := filepath.Join(t.TempDir(), "root-absorbs-terminal-sidecar.htrace")
	input := newScriptedStandaloneInputView(namespace, full)
	binding, err := newProfilerInputBinding(input, namespace)
	if err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	extracted, err := extractProfilerContainerSystraceRowsWithSessionLimitFromInput(
		context.Background(), binding, int64(len(root)), sink)
	if err != nil {
		t.Fatal(err)
	}
	requireProfilerRootProfileFailClosed(t, extracted, sink)
}
