package skill

// AnalysisArtifactValueProfileDescription is shared by the analyzer skill and
// emit schema so optional runtime values retain one applicability/provenance rule.
const AnalysisArtifactValueProfileDescription = "Optional typed profile for a scalar answer explicitly requested from an attached log, trace, perf artifact, or typed runtime observation. Emit it only with predicates.is_scalar_answer=true. It is a soft lookup lane that later stages must verify; never transcribe a pre-triage model summary, stall guess, or inferred root-cause value into this profile."
