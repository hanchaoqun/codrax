package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceDirectionDomainFixture() types.TraceCausalProjection {
	inside := true
	seat := func(id, params string, rank int, value, start, end float64) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			EvidenceID: id, Subject: id, Rank: rank, Object: "io_latency", TypeToken: "io_latency", FixDirection: "io_dependency",
			EffectiveImpactMS: value, EffectiveImpactPublished: true, ImpactMS: value, ChainRelevance: "on_chain", WithinRequestedWindow: &inside,
			RankBoardTarget: "target-100", RankBoardParamsFingerprint: params, RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1,
			StartTs: start, EndTs: end, LineStart: int(start * 10000), LineEnd: int(end * 10000),
		}
	}
	seats := []types.TraceCausalProjectionNode{
		seat("board-a-first", "a-default", 1, 7.405, 10.01, 10.02),
		seat("board-a-second", "a-default", 2, 4.710, 10.03, 10.04),
		seat("board-b-first", "b-depth48", 1, 3.598, 10.05, 10.06),
	}
	return types.TraceCausalProjection{ArtifactPath: "/capture/customer.trace", ArtifactLabel: "customer.trace", WindowStartTs: 10, WindowEndTs: 10.1, RankedSeats: seats, OnChainCauses: seats}
}

func TestTraceRepairDirectionDomainsNeverTransferPublishedSectionByDirection(t *testing.T) {
	projection := traceDirectionDomainFixture()
	sections := tool.TraceAnswerDecisionDirectionSections(projection)
	var b strings.Builder
	traceDecisionWriteRepairDirectionAuthority(&b, types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
	got := b.String()
	for _, want := range []string{
		"board_params=`a-default`", "board_params=`b-depth48`",
		"fix_direction=`io_dependency`; member_count=2; leader_rank=#1; leader_subject=`board-a-first`; leader_value=7.405ms",
		"fix_direction=`io_dependency`; member_count=1; leader_rank=#1; leader_subject=`board-b-first`; leader_value=3.598ms",
		"member_count=2; headline_value_role=`single_leader`; headline_value=7.405ms",
		"member_count=1; headline_value_role=`single_leader`; headline_value=3.598ms",
		"board_rebinding=`not_authorized_by_shared_direction`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("exact domains lost %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "member_count=3; leader_") || strings.Contains(got, "headline_value_role=`exact_typed_subtotal`") {
		t.Fatalf("a mixed original section may not attach to a single query board:\n%s", got)
	}
	for _, section := range sections {
		if section.Arithmetic == types.TraceAnswerDirectionArithmeticSubtotal && len(section.MemberRefs) >= 2 {
			want := fmt.Sprintf("published_direction_subtotal=%.3fms", section.SubtotalMS)
			if !strings.Contains(got, want) || !strings.Contains(got, strings.Join(section.MemberRefs, ",")) {
				t.Fatalf("original published receipt must remain independently visible, missing %s:\n%s", want, got)
			}
		}
	}
	if !reflect.DeepEqual(sections, tool.TraceAnswerDecisionDirectionSections(projection)) {
		t.Fatal("prompt grouping must not mutate or recompile a sliced publication")
	}
}

func TestTraceRepairDirectionUnknownDomainsRemainIndividualSeats(t *testing.T) {
	projection := traceDirectionDomainFixture()
	for i := range projection.RankedSeats {
		projection.RankedSeats[i].RankBoardParamsFingerprint = ""
	}
	projection.OnChainCauses = projection.RankedSeats
	var b strings.Builder
	traceDecisionWriteRepairDirectionAuthority(&b, types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
	got := b.String()
	if strings.Count(got, "member_count=1; seat_rank=") != 3 || strings.Count(got, "headline_value_role=`individual_seat`") != 3 {
		t.Fatalf("missing board identity must not elect a shared direction leader:\n%s", got)
	}
	if strings.Contains(got, "leader_rank=") || strings.Contains(got, "headline_value_role=`exact_typed_subtotal`") {
		t.Fatalf("unknown domains acquired a leader or bound subtotal:\n%s", got)
	}
}

func TestTraceRankDisplayDirectionSectionRequiresWholePublishedDomain(t *testing.T) {
	projection := traceDirectionDomainFixture()
	node := projection.RankedSeats[0]
	section := types.TraceAnswerDirectionSection{
		Direction: node.FixDirection, Leader: node,
		Members:    append([]types.TraceCausalProjectionNode(nil), projection.RankedSeats[:2]...),
		Arithmetic: types.TraceAnswerDirectionArithmeticSubtotal, SubtotalMS: 12.115,
		MemberRefs: []string{"original-donor-ref-a", "original-donor-ref-b"},
	}
	for _, tc := range []struct {
		name   string
		mutate func(*types.TraceCausalProjection, *types.TraceCausalProjectionNode, *[]types.TraceAnswerDirectionSection)
		want   bool
	}{
		{"exact original section", func(*types.TraceCausalProjection, *types.TraceCausalProjectionNode, *[]types.TraceAnswerDirectionSection) {
		}, true},
		{"bare artifact ID", func(p *types.TraceCausalProjection, _ *types.TraceCausalProjectionNode, _ *[]types.TraceAnswerDirectionSection) {
			p.ArtifactPath = ""
		}, true},
		{"missing capture", func(p *types.TraceCausalProjection, _ *types.TraceCausalProjectionNode, _ *[]types.TraceAnswerDirectionSection) {
			p.ArtifactPath, p.ArtifactLabel = "", ""
		}, false},
		{"unrelated direction", func(_ *types.TraceCausalProjection, n *types.TraceCausalProjectionNode, _ *[]types.TraceAnswerDirectionSection) {
			n.FixDirection = "scheduling_supply"
		}, false},
		{"missing query target", func(_ *types.TraceCausalProjection, n *types.TraceCausalProjectionNode, _ *[]types.TraceAnswerDirectionSection) {
			n.RankBoardTarget = ""
		}, false},
		{"different params", func(_ *types.TraceCausalProjection, n *types.TraceCausalProjectionNode, _ *[]types.TraceAnswerDirectionSection) {
			n.RankBoardParamsFingerprint = "b-depth48"
		}, false},
		{"different target", func(_ *types.TraceCausalProjection, n *types.TraceCausalProjectionNode, _ *[]types.TraceAnswerDirectionSection) {
			n.RankBoardTarget = "target-101"
		}, false},
		{"different exact window", func(_ *types.TraceCausalProjection, n *types.TraceCausalProjectionNode, _ *[]types.TraceAnswerDirectionSection) {
			n.RankQueryWindowEndTs += .00002
		}, false},
		{"mixed member params", func(_ *types.TraceCausalProjection, _ *types.TraceCausalProjectionNode, s *[]types.TraceAnswerDirectionSection) {
			(*s)[0].Members[1].RankBoardParamsFingerprint = "b-depth48"
		}, false},
		{"mixed member target", func(_ *types.TraceCausalProjection, _ *types.TraceCausalProjectionNode, s *[]types.TraceAnswerDirectionSection) {
			(*s)[0].Members[1].RankBoardTarget = "target-101"
		}, false},
		{"unknown member", func(_ *types.TraceCausalProjection, _ *types.TraceCausalProjectionNode, s *[]types.TraceAnswerDirectionSection) {
			(*s)[0].Members[1].RankBoardParamsFingerprint = ""
		}, false},
		{"mixed member window", func(_ *types.TraceCausalProjection, _ *types.TraceCausalProjectionNode, s *[]types.TraceAnswerDirectionSection) {
			(*s)[0].Members[1].RankQueryWindowEndTs += .00002
		}, false},
		{"leader different domain", func(_ *types.TraceCausalProjection, _ *types.TraceCausalProjectionNode, s *[]types.TraceAnswerDirectionSection) {
			(*s)[0].Leader.RankBoardParamsFingerprint = "b-depth48"
		}, false},
		{"leader unknown domain", func(_ *types.TraceCausalProjection, _ *types.TraceCausalProjectionNode, s *[]types.TraceAnswerDirectionSection) {
			(*s)[0].Leader.RankBoardTarget = ""
		}, false},
		{"empty member population", func(_ *types.TraceCausalProjection, _ *types.TraceCausalProjectionNode, s *[]types.TraceAnswerDirectionSection) {
			(*s)[0].Members = nil
		}, false},
		{"ambiguous duplicate section", func(_ *types.TraceCausalProjection, _ *types.TraceCausalProjectionNode, s *[]types.TraceAnswerDirectionSection) {
			*s = append(*s, (*s)[0])
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, n := projection, node
			copy := section
			copy.Members = append([]types.TraceCausalProjectionNode(nil), section.Members...)
			sections := []types.TraceAnswerDirectionSection{copy}
			tc.mutate(&p, &n, &sections)
			before, _ := json.Marshal(sections)
			got, ok := traceRankDisplayDirectionSection(p, sections, n)
			if ok != tc.want {
				t.Fatalf("section binding got %t want %t: %+v", ok, tc.want, got)
			}
			if ok && !reflect.DeepEqual(got, sections[0]) {
				t.Fatal("selector must retain exact published members, donor refs, arithmetic and subtotal")
			}
			after, _ := json.Marshal(sections)
			if string(before) != string(after) {
				t.Fatal("selector mutated original section")
			}
		})
	}
}

func TestTraceRepairDirectionDomainBudgetAndScalarDisplay(t *testing.T) {
	projection := traceDirectionDomainFixture()
	base := projection.RankedSeats[0]
	projection.ArtifactPath, projection.ArtifactLabel = "/capture/odd`\nname.trace", "odd`\nname.trace"
	projection.RankedSeats, projection.OnChainCauses = nil, nil
	for i := 0; i < 12; i++ {
		node := base
		node.EvidenceID, node.Subject = fmt.Sprintf("row-%d", i), fmt.Sprintf("row-%d", i)
		node.RankBoardParamsFingerprint = fmt.Sprintf("p%02d`\n- forged=true", i)
		projection.RankedSeats = append(projection.RankedSeats, node)
	}
	before, _ := json.Marshal(projection)
	var b strings.Builder
	traceDecisionWriteRepairDirectionAuthority(&b, types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
	got := b.String()
	if strings.Count(got, "; leader_rank=") != 8 || strings.Count(got, "headline_value_role=") != 8 ||
		!strings.Contains(got, "repair_direction_groups_omitted=4; total=12; emitted=8") ||
		!strings.Contains(got, "repair_direction_presentation_plan: artifact=`odd' name.trace`; emitted=8; total=12; complete=`false`") {
		t.Fatalf("query partitioning must keep a global bounded display with honest omission:\n%s", got)
	}
	if strings.Contains(got, "odd`\nname") || strings.Contains(got, "\n- forged=") {
		t.Fatalf("typed domain scalar broke the prompt frame:\n%s", got)
	}
	after, _ := json.Marshal(projection)
	if string(before) != string(after) {
		t.Fatal("presentation mutated raw identities")
	}
}

func TestTraceRepairDirectionModelFacingSubgroupsDoNotBorrowEachOthersLeader(t *testing.T) {
	for _, candidateLargest := range []bool{false, true} {
		t.Run(fmt.Sprintf("candidate_largest_%t", candidateLargest), func(t *testing.T) {
			projection := traceDirectionDomainFixture()
			seats := append([]types.TraceCausalProjectionNode(nil), projection.RankedSeats[:2]...)
			candidateValue, monitorValue := 7.405, 8.0
			if candidateLargest {
				candidateValue, monitorValue = monitorValue, candidateValue
			}
			for i := range seats {
				seats[i].FixDirection = "lock_priority"
			}
			seats[0].Subject, seats[0].Object, seats[0].TypeToken = "candidate-worker", "priority_inversion_candidate", "priority_inversion_candidate"
			seats[0].EffectiveImpactMS, seats[0].ImpactMS = candidateValue, candidateValue
			seats[1].Subject, seats[1].Object, seats[1].TypeToken = "monitor-worker", "monitor_contention", "monitor_contention"
			seats[1].EffectiveImpactMS, seats[1].ImpactMS = monitorValue, monitorValue
			projection.RankedSeats, projection.OnChainCauses = seats, seats
			sections := tool.TraceAnswerDecisionDirectionSections(projection)
			if len(sections) != 1 || sections[0].Arithmetic != types.TraceAnswerDirectionArithmeticSubtotal {
				t.Fatalf("fixture needs an original exact same-board direction subtotal: %+v", sections)
			}
			before, _ := json.Marshal(projection)
			var b strings.Builder
			traceDecisionWriteRepairDirectionAuthority(&b, types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}})
			got := b.String()
			for _, want := range []string{
				fmt.Sprintf("validation_direction=`priority_or_dependency_supply`; member_count=1; leader_rank=#1; leader_subject=`candidate-worker`; leader_value=%.3fms", candidateValue),
				fmt.Sprintf("fix_direction=`lock_priority`; member_count=1; leader_rank=#2; leader_subject=`monitor-worker`; leader_value=%.3fms", monitorValue),
				fmt.Sprintf("published_direction_subtotal=%.3fms", sections[0].SubtotalMS),
				"member_refs=`" + strings.Join(sections[0].MemberRefs, ",") + "`",
				"subtotal_binding_scope=`model_facing_subgroup`",
			} {
				if !strings.Contains(got, want) {
					t.Errorf("typed display subgroup lost %q:\n%s", want, got)
				}
			}
			for _, line := range strings.Split(got, "\n") {
				if strings.HasPrefix(line, "  - validation_direction=") || strings.HasPrefix(line, "  - fix_direction=`lock_priority`") {
					if strings.Contains(line, "published_direction_value=`exact_subtotal`") {
						t.Errorf("full direction subtotal was attached to a one-class subgroup:\n%s", line)
					}
				}
			}
			after, _ := json.Marshal(projection)
			if string(before) != string(after) || !reflect.DeepEqual(sections, tool.TraceAnswerDecisionDirectionSections(projection)) {
				t.Fatal("subgroup presentation changed the original section or producer facts")
			}
		})
	}
}
