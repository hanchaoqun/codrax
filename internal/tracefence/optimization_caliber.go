package tracefence

// Optimization-potential words describe an existing typed quantity; they do
// not compute it, grant chain membership, or establish a repair's outcome.
// Keep reader displays and model-facing explanations on the same meaning.
const (
	OptimizationPotentialZH  = "估算优化潜力"
	OptimizationPotentialEN  = "modeled potential"
	OptimizationOverviewZH   = "窗内" + OptimizationPotentialZH + "总览"
	OptimizationOverviewEN   = "Modeled-potential overview"
	OptimizationMeaningZH    = "估算优化潜力按既定规则估算；链上依据不等于收益已验证，实际收益需实施后复测。"
	OptimizationMeaningEN    = "Modeled potential is estimated under the stated rules; on-chain evidence does not establish realized benefit, and actual benefit requires post-change measurement."
	OptimizationLowerBoundZH = "既定理想算力模型内的下界，不是实际收益的下界或保证"
	OptimizationLowerBoundEN = "a lower bound within the stated ideal compute model, not a lower bound or guarantee of actual benefit"
	OptimizationAdjacentZH   = "假定因果关系成立时的模型内潜力边界，不是实际收益上界"
	OptimizationAdjacentEN   = "a within-model potential bound conditional on the causal relation, not an upper bound on actual benefit"
	OptimizationOverlapZH    = "仅证共享计量时间，潜力不可相加；修复收益需复测"
	OptimizationOverlapEN    = "shared measured time only; potentials do not add; repair benefit needs measurement"
)
