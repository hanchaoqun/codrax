package tool

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC232EvidenceIndexUsesTypedChannelAndValueRuler(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, caliber := range ioCaliberRenderCases() {
			for _, channel := range []string{"on_chain", "adjacent", "background", "", "future_unknown"} {
				n := types.TraceCausalProjectionNode{Subject: "issuer-42", Object: "io_latency", TypeToken: "io_latency", IOValueCaliber: caliber, Role: types.TraceCausalRoleRootCauseContext, Rank: 1, ChainRelevance: channel, ResourceCompletionClosure: true}
				before := n
				face := runtimeTraceCausalProjectionEvidenceDisplayDetail(n, zh, false)
				if !strings.Contains(face, types.TraceIOValueCaliberLabel(caliber, zh)) {
					t.Errorf("index lost own ruler: %s", face)
				}
				root := strings.Contains(face, "根因排序") || strings.Contains(face, "root-cause rank")
				if root != (channel == "on_chain") {
					t.Errorf("index promoted/lost typed channel %q: %s", channel, face)
				}
				adjacent := strings.Contains(face, "邻近影响") || strings.Contains(face, "adjacent-impact")
				if adjacent != (channel == "adjacent") {
					t.Errorf("index adjacent channel mismatch %q: %s", channel, face)
				}
				if !reflect.DeepEqual(n, before) {
					t.Fatal("display changed source node")
				}
			}
		}
		for _, tier := range []string{types.TraceCausalTierTargetSelfState, types.TraceCausalTierContextOnly, types.TraceCausalTierCaliberSide, types.TraceCausalTierDataGap} {
			n := types.TraceCausalProjectionNode{TypeToken: "io_latency", IOValueCaliber: types.TraceIOValueCaliberIssuerBlocked, Rank: 1, ChainRelevance: "on_chain", Tier: tier}
			face := runtimeTraceCausalProjectionEvidenceDisplayDetail(n, zh, false)
			if strings.Contains(face, "根因排序") || strings.Contains(face, "root-cause rank") {
				t.Errorf("noncausal tier %s acquired ordinal: %s", tier, face)
			}
		}
		if got := TraceMeasurementTypeDisplayLabel("running", types.TraceIOValueCaliberRQResidence, zh); got != TraceRootCauseTypeDisplayLabel("running", zh) {
			t.Fatal("IO field relabeled an unrelated family")
		}
		if got := TraceMeasurementTypeDisplayLabel("io_wait", "", zh); got != TraceRootCauseTypeDisplayLabel("io_wait", zh) {
			t.Fatal("legacy scheduler IO state lost its own label")
		}
	}
}
