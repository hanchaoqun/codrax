package types

import "testing"

func TestBuildTraceTargetStateScopeAuthoritiesPreservesThreadLocalScope(t *testing.T) {
	set := TraceCausalProjectionSet{Projections: []TraceCausalProjection{
		{
			ArtifactPath:  "/tmp/tieba.systrace",
			ArtifactLabel: "tieba.systrace",
			TargetStateAccount: &TraceCausalProjectionTargetStateAccount{
				Subject:       "com.baidu.tieba-59566",
				WindowStartTs: 34579.472865,
				WindowEndTs:   34579.587805,
				RunningMS:     26.946,
				RunnableMS:    3.636,
				SleepMS:       84.358,
				TotalMS:       114.940,
				EvidenceID:    "target-state",
			},
		},
		{
			ArtifactPath: "/tmp/empty.systrace",
		},
	}}

	got := BuildTraceTargetStateScopeAuthorities(set)
	if len(got) != 1 {
		t.Fatalf("authority count=%d, want 1: %+v", len(got), got)
	}
	if got[0].Subject != "com.baidu.tieba-59566" ||
		got[0].RunningMS != 26.946 ||
		got[0].RunnableMS != 3.636 ||
		got[0].TotalMS != 114.940 {
		t.Fatalf("thread-local account drifted: %+v", got[0])
	}
}
