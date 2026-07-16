package hitraceconv

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestRawPerfMappingContainmentBoundaries(t *testing.T) {
	tests := []struct {
		name              string
		mapping           rawPerfMapping
		ip                uint64
		wantContains      bool
		wantMalformedNear bool
	}{
		{
			name:         "ordinary lower edge",
			mapping:      rawPerfMapping{Addr: 0x1000, Len: 0x20},
			ip:           0x1000,
			wantContains: true,
		},
		{
			name:         "ordinary upper byte",
			mapping:      rawPerfMapping{Addr: 0x1000, Len: 0x20},
			ip:           0x101f,
			wantContains: true,
		},
		{
			name:    "ordinary half open upper edge",
			mapping: rawPerfMapping{Addr: 0x1000, Len: 0x20},
			ip:      0x1020,
		},
		{
			name:    "ordinary below lower edge",
			mapping: rawPerfMapping{Addr: 0x1000, Len: 0x20},
			ip:      0x0fff,
		},
		{
			name:    "zero length never contains",
			mapping: rawPerfMapping{Addr: 0x1000, Len: 0},
			ip:      0x1000,
		},
		{
			name:         "exact address space tail is legal",
			mapping:      rawPerfMapping{Addr: math.MaxUint64 - 31, Len: 32},
			ip:           math.MaxUint64,
			wantContains: true,
		},
		{
			name:              "range extending past address space is relevant and malformed",
			mapping:           rawPerfMapping{Addr: math.MaxUint64 - 31, Len: 33},
			ip:                math.MaxUint64,
			wantMalformedNear: true,
		},
		{
			name:    "malformed distant range does not taint unrelated ip",
			mapping: rawPerfMapping{Addr: math.MaxUint64 - 31, Len: 33},
			ip:      0x1000,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contains, malformedRelevant := rawPerfMappingContainment(test.mapping, test.ip)
			if contains != test.wantContains || malformedRelevant != test.wantMalformedNear {
				t.Fatalf("containment=(%t,%t), want (%t,%t) for mapping=%+v ip=%#x",
					contains, malformedRelevant, test.wantContains, test.wantMalformedNear, test.mapping, test.ip)
			}
		})
	}
}

func TestRawPerfResolverNearMaxTailDoesNotFallIntoDirectPCFallback(t *testing.T) {
	const tailPath = "/system/lib64/libtail.so"
	ip := uint64(math.MaxUint64)
	data := rawPerfData{
		Mappings: []rawPerfMapping{{
			PID: 7, TID: 8, Addr: math.MaxUint64 - 31, Len: 32, Path: tailPath,
		}},
		Features: rawPerfFeatures{SymbolFiles: []rawPerfSymbolFile{
			{
				Path:       tailPath,
				SymbolType: rawPerfSymbolFileELF,
				Symbols: []rawPerfSymbol{{
					Vaddr: 31, Len: 1, Name: "tail_symbol",
				}},
			},
			{
				Path:       "wrong.hap",
				SymbolType: rawPerfSymbolFileHAP,
				Symbols: []rawPerfSymbol{{
					// A zero-length exact symbol deliberately bypasses the
					// independent symbol-end overflow guard. It is an attractive
					// fallback only when tail containment is computed incorrectly.
					Vaddr: ip, Len: 0, Name: "wrong_direct_symbol",
				}},
			},
		}},
	}

	frame := rawPerfResolveFrame(&data, rawPerfSample{PID: 7, TID: 8}, ip)
	if frame.DSO != tailPath || frame.Symbol != "tail_symbol" || !frame.Symbolized || frame.ResolverDegraded {
		t.Fatalf("legal address-space tail did not retain its mapping authority: %+v", frame)
	}
}

func TestRawPerfResolverMalformedMappingIsPartialAndCannotOpenDirectFallback(t *testing.T) {
	ip := uint64(math.MaxUint64)
	data := rawPerfData{
		Mappings: []rawPerfMapping{{
			PID: 17, TID: 18, Addr: math.MaxUint64 - 31, Len: 33, Path: "overflow.so",
		}},
		Features: rawPerfFeatures{SymbolFiles: []rawPerfSymbolFile{{
			Path:       "wrong.hap",
			SymbolType: rawPerfSymbolFileHAP,
			Symbols:    []rawPerfSymbol{{Vaddr: ip, Len: 0, Name: "wrong_direct_symbol"}},
		}}},
	}

	frame := rawPerfResolveFrame(&data, rawPerfSample{PID: 17, TID: 18}, ip)
	if frame.Symbolized || frame.Symbol != rawPerfIP(ip) || frame.DSO != "unknown" || !frame.ResolverDegraded {
		t.Fatalf("malformed relevant mapping acquired resolver metadata instead of a partial verdict: %+v", frame)
	}
}

func TestRawPerfResolverHealthyMappingSiblingClearsMalformedCandidate(t *testing.T) {
	ip := uint64(math.MaxUint64)
	malformed := rawPerfMapping{
		PID: 19, TID: 20, Addr: math.MaxUint64 - 31, Len: 33, Path: "overflow.so",
	}
	healthy := rawPerfMapping{
		PID: 19, TID: 20, Addr: math.MaxUint64 - 31, Len: 32, Path: "healthy-tail.so",
	}
	for _, test := range []struct {
		name     string
		mappings []rawPerfMapping
	}{
		{name: "malformed before healthy", mappings: []rawPerfMapping{malformed, healthy}},
		{name: "healthy before malformed", mappings: []rawPerfMapping{healthy, malformed}},
	} {
		t.Run(test.name, func(t *testing.T) {
			best, ok, degraded := rawPerfBestMapping(test.mappings, rawPerfSample{PID: 19, TID: 20}, ip)
			if !ok || degraded || best.Path != healthy.Path {
				t.Fatalf("healthy mapping sibling did not retain authority: best=%+v ok=%t degraded=%t", best, ok, degraded)
			}
		})
	}
}

func TestRawPerfResolverCheckedTranslationArithmetic(t *testing.T) {
	t.Run("pgoff overflow cannot select wrapped symbol", func(t *testing.T) {
		mapping := rawPerfMapping{
			PID: 21, TID: 22, Addr: 0x1000, Len: 0x100,
			Pgoff: math.MaxUint64 - 16, Path: "libpgoff.so",
		}
		files := []rawPerfSymbolFile{{
			Path:       mapping.Path,
			SymbolType: rawPerfSymbolFileELF,
			Symbols: []rawPerfSymbol{{
				// (0x1020-0x1000)+(MaxUint64-16) wraps to 15.
				Vaddr: 15, Len: 1, Name: "wrapped_pgoff_symbol",
			}},
		}}

		symbol, ok, degraded := rawPerfSymbolForIP(files, mapping, 0x1020)
		if ok || symbol != (rawPerfSymbol{}) || !degraded {
			t.Fatalf("Pgoff overflow was not rejected as degraded metadata: symbol=%+v ok=%t degraded=%t", symbol, ok, degraded)
		}
	})

	t.Run("text exec overflow skips malformed candidate and resolves healthy sibling", func(t *testing.T) {
		mapping := rawPerfMapping{PID: 31, TID: 32, Addr: 0x1000, Len: 0x100, Path: "libtext.so"}
		files := []rawPerfSymbolFile{
			{
				Path:          mapping.Path,
				SymbolType:    rawPerfSymbolFileELF,
				TextExecVaddr: math.MaxUint64 - 16,
				Symbols: []rawPerfSymbol{{
					// 0x20+(MaxUint64-16) wraps to 15.
					Vaddr: 15, Len: 1, Name: "wrapped_text_symbol",
				}},
			},
			{
				Path:       mapping.Path,
				SymbolType: rawPerfSymbolFileELF,
				Symbols: []rawPerfSymbol{{
					Vaddr: 0x20, Len: 1, Name: "healthy_sibling_symbol",
				}},
			},
		}

		symbol, ok, degraded := rawPerfSymbolForIP(files, mapping, 0x1020)
		if !ok || degraded || symbol.Name != "healthy_sibling_symbol" {
			t.Fatalf("malformed symbol file poisoned a healthy same-path sibling: symbol=%+v ok=%t degraded=%t", symbol, ok, degraded)
		}
	})
}

func TestRawPerfResolverDirectPCDoesNotUseMappedAddressTranslation(t *testing.T) {
	const ip = uint64(0x7020)
	file := rawPerfSymbolFile{
		Path:                    "entry.hap",
		SymbolType:              rawPerfSymbolFileHAP,
		TextExecVaddr:           math.MaxUint64,
		TextExecVaddrFileOffset: math.MaxUint64,
		Symbols:                 []rawPerfSymbol{{Vaddr: ip, Len: 1, Name: "direct_pc_symbol"}},
	}
	mapping := rawPerfMapping{
		PID: 41, TID: 42, Addr: 0x7000, Len: 0x100,
		Pgoff: math.MaxUint64, Path: file.Path,
	}

	symbol, ok, degraded := rawPerfSymbolForIP([]rawPerfSymbolFile{file}, mapping, ip)
	if !ok || degraded || symbol.Name != "direct_pc_symbol" {
		t.Fatalf("direct-PC symbol was incorrectly sent through mapped-address arithmetic: symbol=%+v ok=%t degraded=%t", symbol, ok, degraded)
	}

	withoutMapping := rawPerfData{Features: rawPerfFeatures{SymbolFiles: []rawPerfSymbolFile{file}}}
	frame := rawPerfResolveFrame(&withoutMapping, rawPerfSample{PID: 41, TID: 42}, ip)
	if !frame.Symbolized || frame.ResolverDegraded || frame.Symbol != "direct_pc_symbol" || frame.DSO != file.Path {
		t.Fatalf("healthy direct-PC fallback regressed: %+v", frame)
	}
}

func TestWriteRawPerfResolverFailurePreservesSampleIdentityAndHealthySibling(t *testing.T) {
	badIP := uint64(math.MaxUint64)
	goodIP := uint64(0x2020)
	data := rawPerfData{
		Mappings: []rawPerfMapping{
			{PID: 101, TID: 102, Addr: math.MaxUint64 - 31, Len: 33, Path: "overflow.so"},
			{PID: 201, TID: 202, Addr: 0x2000, Len: 0x100, Path: "healthy.so"},
		},
		Features: rawPerfFeatures{SymbolFiles: []rawPerfSymbolFile{{
			Path:       "healthy.so",
			SymbolType: rawPerfSymbolFileELF,
			Symbols:    []rawPerfSymbol{{Vaddr: 0x20, Len: 1, Name: "healthy_symbol"}},
		}}},
		Samples: []rawPerfSample{
			{
				PID: 101, TID: 102, Comm: "bad-range", CPU: 3, CPUValid: true,
				TimeNS: 1_000_000_000, IP: badIP, Period: 7, EventName: "cpu-cycles",
			},
			{
				PID: 201, TID: 202, Comm: "healthy", CPU: 4, CPUValid: true,
				TimeNS: 2_000_000_000, IP: goodIP, Period: 11, EventName: "instructions",
			},
		},
	}

	var wire bytes.Buffer
	if err := writeRawPerfDataPerfTrace(context.Background(), &wire, data); err != nil {
		t.Fatalf("write resolver boundary fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "resolver-range.perftrace")
	if err := os.WriteFile(path, wire.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	index, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("parse resolver boundary fixture: %v", err)
	}
	if len(index.Events) != 2 {
		t.Fatalf("resolver metadata failure dropped or duplicated a core sample: got %d events\n%s", len(index.Events), wire.String())
	}

	bad := index.Events[0]
	if bad.Type != tracequery.EventPerfSample || bad.Ts != 1 || bad.CPU != 3 || bad.PerfFields == nil ||
		bad.PerfFields.PID != 101 || bad.PerfFields.TID != 102 || bad.PerfFields.Period != 7 ||
		bad.PerfFields.EventName != "cpu-cycles" || bad.PerfFields.IP != rawPerfIP(badIP) ||
		bad.PerfFields.Symbol != rawPerfIP(badIP) || bad.PerfFields.SymbolizationStatus != "partial" ||
		bad.PerfFields.CallchainStatus != "partial" {
		t.Fatalf("resolver failure corrupted core sample identity or its partial verdict: %+v\n%s", bad, wire.String())
	}

	healthy := index.Events[1]
	if healthy.Type != tracequery.EventPerfSample || healthy.Ts != 2 || healthy.CPU != 4 || healthy.PerfFields == nil ||
		healthy.PerfFields.PID != 201 || healthy.PerfFields.TID != 202 || healthy.PerfFields.Period != 11 ||
		healthy.PerfFields.EventName != "instructions" || healthy.PerfFields.IP != rawPerfIP(goodIP) ||
		healthy.PerfFields.Symbol != "healthy_symbol" || healthy.PerfFields.DSO != "healthy.so" ||
		healthy.PerfFields.SymbolizationStatus != "symbolized" || healthy.PerfFields.CallchainStatus != "symbolized" {
		t.Fatalf("malformed sibling tainted healthy symbol resolution: %+v\n%s", healthy, wire.String())
	}
}
