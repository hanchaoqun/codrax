package tool

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

var preparedFrameRequestRE = regexp.MustCompile(`(?m)^目标 Trace: ([^\r\n]+?)；(线程=([0-9]+))；帧窗口=(([0-9]+(?:\.[0-9]{1,9})?)\.\.([0-9]+(?:\.[0-9]{1,9})?)秒)。(?:\r?$)`)

// Bind the existing single-frame request envelope to its attached sources.
// Only coordinates authored in that envelope and corroborated by the complete
// fact record may override the analyzer's classification. A fact alone supplies
// neither user scope nor a causal conclusion.
func bindPreparedFrameRequest(ctx *types.BusContext, raw string, p *emitAnalysisParams) bool {
	fact, ok := ctx.AttachedLogCatalog.PreparedFrameFact()
	if !ok || !ctx.AttachedTraceMaterial.SinglePhysicalSource() {
		return false
	}
	firstLine, _, _ := strings.Cut(strings.TrimSpace(raw), "\n")
	if strings.TrimSpace(firstLine) != "# 单帧原因调查" || strings.Count(raw, "；帧窗口=") != 1 {
		return false
	}
	match := preparedFrameRequestRE.FindStringSubmatch(raw)
	if match == nil || !ctx.AttachedTraceMaterial.MatchesPath(match[1]) {
		return false
	}
	tid, err := strconv.Atoi(match[3])
	if err != nil || tid != fact.TID {
		return false
	}
	start, startOK := new(big.Rat).SetString(match[5])
	end, endOK := new(big.Rat).SetString(match[6])
	if !startOK || !endOK || start.Cmp(big.NewRat(fact.StartNS, 1e9)) != 0 ||
		end.Cmp(big.NewRat(fact.EndNS, 1e9)) != 0 {
		return false
	}
	startSeconds, _ := start.Float64()
	endSeconds, _ := end.Float64()
	confidence := 1.0
	p.RuntimeArtifactScopeProfile = &emitRuntimeArtifactScopeProfileParam{
		RequestedScope: string(types.RuntimeArtifactScopeExplicitWindow),
		TimeStart:      &startSeconds, TimeEnd: &endSeconds, SourceQuote: match[4], Confidence: &confidence,
	}
	p.RuntimeTargetProfile = &emitRuntimeTargetProfileParam{
		Declaration: string(types.RuntimeTargetDeclarationNamedTarget),
		SourceQuote: match[2], Confidence: &confidence,
	}
	p.RuntimeTargets = []emitRuntimeTargetParam{{
		Kind: string(types.RuntimeTargetKindThread), PID: &tid, Source: "user_explicit", Confidence: &confidence,
	}}
	return true
}
