package types

import "testing"

func TestRuntimeArtifactWorkCopyHMC291(t *testing.T) {
	p := &RuntimeArtifactWorkProfile{Mode: RuntimeArtifactWorkRepresentative, PerSpecificationLimit: 2, SourceQuote: "每组两份"}
	m := NewMutableState("每组两份")
	m.SetRequestModel(RequestModel{RuntimeArtifactWorkProfile: p})
	p.PerSpecificationLimit = 7
	if m.RequestModel().RuntimeArtifactWorkProfile.PerSpecificationLimit != 2 {
		t.Fatal("SetRequestModel aliases caller profile")
	}
	fork := m.ForkForExploreDispatch()
	fork.requestModel.RuntimeArtifactWorkProfile.PerSpecificationLimit = 3
	if m.RequestModel().RuntimeArtifactWorkProfile.PerSpecificationLimit != 2 {
		t.Fatal("exploration fork aliases parent policy")
	}
	snapshot := m.RequestModel()
	snapshot.RuntimeArtifactWorkProfile.SourceQuote = "mutated"
	if m.RequestModel().RuntimeArtifactWorkProfile.SourceQuote != "每组两份" {
		t.Fatal("RequestModel aliases stored provenance")
	}
}
