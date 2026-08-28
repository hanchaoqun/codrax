package agent

// runtimeArtifactObservationCausalityCalibration is shared soft guidance for
// observation-only runtime artifacts. It deliberately describes evidence
// authority rather than selecting a conclusion: later source/artifact rows can
// still prove a producer or mechanism, while a bare stack/status observation
// cannot. Callers must only render this after typed runtime-artifact authority
// has selected the observation-only lane.
func runtimeArtifactObservationCausalityCalibration() string {
	return "- A stack frame, its rendered argument/receiver values, and within-stack order prove only what was observed at that frame and which frames were present. They do not by themselves prove which caller created or supplied a value, value ownership, a missing initialization/guard, or the upstream construction path.\n" +
		"- A runtime error or status proves that observed status at the named frame/time. It does not by itself prove the entry-time state, caller policy/configuration, downstream slowness, or one specific causal mechanism. Present those as follow-up hypotheses unless another typed artifact row or grounded current-source operation independently proves them.\n"
}
