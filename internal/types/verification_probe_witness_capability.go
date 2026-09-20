package types

// VerificationProbeUsesExecutionOnlyWitness is the shared authority ceiling
// for inline runtimes whose executor-owned receipt proves changed-target
// execution, but not each declared contract or placement assertion. It is not
// a runtime-support/availability predicate: callers selecting a runtime must
// still use the existing runtime registry and target-language compatibility.
// Keep dispatch and the actual receipt/confidence producers on this predicate.
func VerificationProbeUsesExecutionOnlyWitness(language string) bool {
	return VerificationProbeLanguageIsPython(language)
}
