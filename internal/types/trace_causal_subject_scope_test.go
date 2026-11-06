package types

import "testing"

func TestHMC363RankSubjectScopeSeparatesMemberAndRequest(t *testing.T) {
	const tid, tgid, upstream = 42, 400, 99
	native := TraceCausalProjectionNode{RankSubjects: &TraceRankSubjects{
		Target: TraceThreadCoordinate{TID: tid, TGID: tgid, Comm: "current label"},
		Member: TraceThreadCoordinate{TID: upstream, TGID: -1, Comm: "upstream"},
	}}
	for _, tc := range []struct {
		name        string
		declaration RuntimeTargetDeclaration
		target      RuntimeTarget
		want        TraceRankRequestBinding
	}{
		{"no declaration", "", RuntimeTarget{}, ""},
		{"no named target", RuntimeTargetDeclarationNoNamedTarget, RuntimeTarget{Kind: RuntimeTargetKindThread, PID: tid, Source: "user_explicit"}, TraceRankRequestUnbound},
		{"thread match", RuntimeTargetDeclarationNamedTarget, RuntimeTarget{Kind: RuntimeTargetKindThread, PID: tid, Source: "user_explicit"}, TraceRankRequestedThreadID},
		{"process match", RuntimeTargetDeclarationNamedTarget, RuntimeTarget{Kind: RuntimeTargetKindProcess, PID: tgid, Source: "user_explicit"}, TraceRankRequestedProcessID},
		{"member is not target", RuntimeTargetDeclarationNamedTarget, RuntimeTarget{Kind: RuntimeTargetKindThread, PID: upstream, Source: "user_explicit"}, TraceRankRequestUnbound},
		{"thread ID is not process ID", RuntimeTargetDeclarationNamedTarget, RuntimeTarget{Kind: RuntimeTargetKindProcess, PID: tid, Source: "user_explicit"}, TraceRankRequestUnbound},
		{"exploration cursor", RuntimeTargetDeclarationNamedTarget, RuntimeTarget{Kind: RuntimeTargetKindThread, PID: tid, Source: RuntimeTargetSourceExplicitToolCall}, TraceRankRequestUnbound},
		{"name alone", RuntimeTargetDeclarationNamedTarget, RuntimeTarget{Kind: RuntimeTargetKindThread, Thread: "current label", Source: "user_explicit"}, TraceRankRequestUnbound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := &RuntimeTargetProfile{Declaration: tc.declaration, SourceQuote: "validated user anchor"}
			if tc.declaration == "" {
				profile = nil
			}
			projection := TraceCausalProjection{rankUserFocus: traceRankUserFocusFromRequest(&RequestModel{
				RuntimeTargetProfile: profile, RuntimeTargets: []RuntimeTarget{tc.target},
			})}
			scope := projection.RankSubjectScope(native)
			if tc.want == "" {
				if scope != nil {
					t.Fatalf("legacy request acquired identity authority: %+v", scope)
				}
				return
			}
			if scope == nil || scope.Binding != tc.want || scope.Target != native.RankSubjects.Target || scope.Member.TID != upstream || scope.Member.TGID != 0 || scope.Member.Comm != "upstream" {
				t.Fatalf("wrong scope: %+v", scope)
			}
			if s := projection.RankSubjectScope(TraceCausalProjectionNode{}); s != nil {
				t.Fatalf("legacy observation acquired identity authority: %+v", s)
			}
		})
	}
}
