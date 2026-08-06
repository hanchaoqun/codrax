package dataquery

// actionRunnerCustomTransformOutputContract keeps intermediate transforms
// relaxed so an ordinary artifact payload is never mistaken for the user's
// answer. A batch that the shared validation policy already recognizes as
// terminal, however, executes under that same output contract. This lets
// Runner losslessly promote a plain JSON payload from emit({...}) or a result
// assignment without publishing a strict contract beside an empty answer.
// The decision is structural (plan/stage facts), not payload prose.
func actionRunnerCustomTransformOutputContract(plan TaskPlan, seed Result) OutputContract {
	terminalPlan := actionRunnerValidationPlan(plan, seed)
	contract := terminalPlan.OutputContract.Normalize()
	if contract.Format != OutputFreeform {
		return contract
	}
	return OutputContract{Format: OutputFreeform, ExplanationAllowed: true}
}
