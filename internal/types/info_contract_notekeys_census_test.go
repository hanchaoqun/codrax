package types

// info_contract_notekeys_census_test.go — UXG-1 T2 armA (ledger §29.40;
// mechanization spec info_contract_report.md §④): note-key CARRIER TRUTH —
// the registry's carrier column is checked against the actual projection
// compile, killing 病理形B/形C on the wire→Node link (the OM-10 假注释形:
// a display_only key whose comment claimed a compile consumer).
//
// Arms (precise signals only; comments stripped before scanning):
//   1. hard_consumer / anchor_window keys MUST be referenced by the
//      projection-compile sources (internal/types non-test, minus the
//      registry file itself) — via their TraceNoteKey* constant or the
//      quoted literal. A hard key the compile never reads = silent
//      promotion loss (the carrier column lies).
//   2. display_only keys must NOT be referenced there — a compile parse of a
//      display_only key is 越权 (promote through the NKR protocol instead).
//      To keep 报红必真 (generic short keys like "state"/"kind" collide with
//      unrelated literals), this negative arm applies only to keys that have
//      a TraceNoteKey* constant or an explicit ledger row below.
//   3. The OM/W-implicated display_only keys are LEDGERED below (known-gap
//      keys of the §29.40 matrix). Each must exist in the registry and still
//      be display_only TODAY: when a fix batch (IC-A/IC-L/…) promotes the
//      key, this census reddens until the ledger row is retired together
//      with the contract flip (账实一致纪律). Non-ledgered display_only keys
//      are LLM-face wire disclosure by carrier definition — their ledger
//      entry IS the registry row (single source, no second copy).
//   4. soft_consumer keys must still be referenced SOMEWHERE in the
//      engine/display packages (a fully dead soft row reddens; producer
//      sites can false-green this arm — documented, conservative direction).

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// infoContractNoteKeyLedger — the §29.40 known-gap / exemption note keys
// (display_only today, host batch owns the flip).
var infoContractNoteKeyLedger = map[string]string{
	"drill_status":                "OM-7 → IC-A(投影头部强制披露半场)",
	"edge_count":                  "OM-9 → IC-A(邻近判据)",
	"nearest_chain_thread":        "OM-9 → IC-A",
	"holder_ns_unification":       "OM-10 → IC-L",
	"holder_host_process":         "OM-10 → IC-L",
	"wait_object":                 "OM-11 → IC-L(等待对象独立键行)",
	"peer_state_dominant":         "OM-11 → IC-L(对端状态行)",
	"peer_state_total":            "OM-11 → IC-L",
	"peer_state_running":          "OM-11 → IC-L",
	"peer_state_runnable":         "OM-11 → IC-L",
	"peer_state_sleep":            "OM-11 → IC-L",
	"peer_state_d_state":          "OM-11 → IC-L",
	"peer_state_io_wait":          "OM-11 → IC-L",
	"peer_state_fragments":        "OM-11 → IC-L",
	"peer_chain_state":            "OM-11 伴生 → IC-L(对端续链方向)",
	"peer_chain_blocker":          "OM-11 伴生 → IC-L",
	"peer_chain_blocker_state":    "OM-11 伴生 → IC-L",
	"peer_chain_blocker_source":   "OM-11 伴生 → IC-L",
	"peer_chain_presumptive":      "OM-11 伴生 → IC-L",
	"inherited_target_blocked_ms": "OM-13 → IC-A(承接注记)",
	"priority_inversion_gated":    "W-20(理想源;接线留 P0-E,campaign:1272)",
}

func infoContractReadSources(t *testing.T, dir string, skip map[string]bool) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || skip[name] {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if at := strings.Index(line, "\t//"); at >= 0 {
				line = line[:at]
			} else if at := strings.Index(line, " //"); at >= 0 {
				line = line[:at]
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// infoContractNoteKeyConstants parses the registry file's exported
// TraceNoteKey* constant declarations into key → constant-names. A key may
// be carried by several constants: its own TraceNoteKey* plus any composite
// TraceNoteMarker* built FROM that constant (the ledger-marker family — e.g.
// observation_ledger.go parses TraceNoteMarkerAdvisoryPretriage back, which
// IS the advisory_pretriage consumer).
// Two tiers: direct = the key's own TraceNoteKey* constant (a compile PARSE
// references this form); markerDerived additionally includes composite
// TraceNoteMarker* constants built from the key (ledger-marker stamps —
// legitimate for the PRESENCE arms, but a stamp write must not trip the
// display_only negative arm: TraceNoteMarkerLegacySummaryFallback is
// write-only in the compile and its keys stay truthfully display_only).
func infoContractNoteKeyConstants(t *testing.T) (direct, markerDerived map[string][]string) {
	t.Helper()
	raw, err := os.ReadFile("trace_note_keys.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	direct = map[string][]string{}
	markerDerived = map[string][]string{}
	nameByConst := map[string]string{}
	re := regexp.MustCompile(`(TraceNoteKey\w+)\s*=\s*"([a-z0-9_]+)"`)
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		direct[m[2]] = append(direct[m[2]], m[1])
		markerDerived[m[2]] = append(markerDerived[m[2]], m[1])
		nameByConst[m[1]] = m[2]
	}
	marker := regexp.MustCompile(`(TraceNoteMarker\w+)\s*=\s*(.+)`)
	for _, m := range marker.FindAllStringSubmatch(src, -1) {
		for constName, key := range nameByConst {
			if strings.Contains(m[2], constName) {
				markerDerived[key] = append(markerDerived[key], m[1])
			}
		}
	}
	return direct, markerDerived
}

func TestInfoContractNoteKeyCarrierTruth(t *testing.T) {
	compileSrc := infoContractReadSources(t, ".", map[string]bool{"trace_note_keys.go": true})
	broadSrc := compileSrc +
		infoContractReadSources(t, "../tool", nil) +
		infoContractReadSources(t, "../tracequery", nil) +
		infoContractReadSources(t, "../agent", nil)
	direct, markerDerived := infoContractNoteKeyConstants(t)

	referencedVia := func(src, key string, constants map[string][]string) bool {
		for _, constName := range constants[key] {
			if regexp.MustCompile(`\b` + constName + `\b`).MatchString(src) {
				return true
			}
		}
		return strings.Contains(src, `"`+key+`"`)
	}
	referenced := func(src, key string) bool { return referencedVia(src, key, markerDerived) }

	seenLedger := map[string]bool{}
	for _, row := range TraceNoteKeyRows() {
		switch row.Carrier {
		case TraceNoteCarrierHardConsumer, TraceNoteCarrierAnchorWindow:
			if !referenced(compileSrc, row.Key) {
				t.Errorf("键 %q 注册 %s 但投影编译零读(OM-10 假注释形:载体列撒谎或消费者被删)", row.Key, row.Carrier)
			}
		case TraceNoteCarrierDisplayOnly:
			hasConst := len(direct[row.Key]) > 0
			_, ledgered := infoContractNoteKeyLedger[row.Key]
			if (hasConst || ledgered) && referencedVia(compileSrc, row.Key, direct) {
				t.Errorf("键 %q 注册 display_only 却被投影编译引用(越权——经 NKR 协议升格并同步契约/账本)", row.Key)
			}
			if ledgered {
				seenLedger[row.Key] = true
			}
		case TraceNoteCarrierSoftConsumer:
			if !referenced(broadSrc, row.Key) {
				t.Errorf("键 %q 注册 soft_consumer 但引擎/显示包零引用(死行)", row.Key)
			}
		}
	}
	for key, ref := range infoContractNoteKeyLedger {
		if !seenLedger[key] {
			row, ok := TraceNoteKeyLookup(key)
			switch {
			case !ok:
				t.Errorf("known-gap 账目键 %q 已不在注册表(同步销账:%s)", key, ref)
			case row.Carrier != TraceNoteCarrierDisplayOnly:
				t.Errorf("known-gap 账目键 %q 已升格为 %s——修复批落地,账目行须随契约翻状态后删除(%s)", key, row.Carrier, ref)
			default:
				t.Errorf("known-gap 账目键 %q 未被 census 扫到(注册表遍历缺口)", key)
			}
		}
	}
}
