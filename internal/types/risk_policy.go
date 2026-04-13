package types

// RunPolicy is the pipeline policy materialized after analyze and then frozen
// for the rest of the run.
type RunPolicy struct {
	RequireReview       bool            `json:"require_review" yaml:"require_review"`
	RequireVerification bool            `json:"require_verification" yaml:"require_verification"`
	MandatoryStages     []PipelineStage `json:"mandatory_stages,omitempty" yaml:"mandatory_stages,omitempty"`
}

type RiskPolicyRule struct {
	Name            string
	Match           func(RiskMatrix) bool
	RunPolicy       RunPolicy
	MinRiskLabel    string
	EscalationScore int
}

var RiskPolicyRules = []RiskPolicyRule{
	{
		Name: "critical-security-or-integrity",
		Match: func(m RiskMatrix) bool {
			return m.Security.Level >= 4 || m.DataIntegrity.Level >= 4 || m.Compliance.Level >= 4
		},
		RunPolicy: RunPolicy{
			RequireReview:       true,
			RequireVerification: true,
			MandatoryStages:     []PipelineStage{StagePlan, StageDesignReview, StageCodeReview, StageVerify},
		},
		MinRiskLabel:    "critical",
		EscalationScore: 4,
	},
	{
		Name: "high-compatibility-performance-ops",
		Match: func(m RiskMatrix) bool {
			return m.Compatibility.Level >= 4 || m.Performance.Level >= 4 || m.Ops.Level >= 4
		},
		RunPolicy: RunPolicy{
			RequireReview:       false,
			RequireVerification: true,
			MandatoryStages:     []PipelineStage{StagePlan, StageVerify},
		},
		MinRiskLabel:    "high",
		EscalationScore: 4,
	},
	{
		Name: "elevated-any-dimension",
		Match: func(m RiskMatrix) bool {
			return maxRiskLevel(m) >= 3
		},
		RunPolicy: RunPolicy{
			RequireReview:       false,
			RequireVerification: true,
			MandatoryStages:     []PipelineStage{StagePlan, StageVerify},
		},
		MinRiskLabel:    "elevated",
		EscalationScore: 3,
	},
}

func ResolveRiskPolicy(m RiskMatrix) RunPolicy {
	for _, rule := range RiskPolicyRules {
		if rule.Match != nil && rule.Match(m) {
			return rule.RunPolicy
		}
	}
	return RunPolicy{}
}

func maxRiskLevel(m RiskMatrix) int {
	levels := []int{
		m.Security.Level,
		m.DataIntegrity.Level,
		m.Compatibility.Level,
		m.Performance.Level,
		m.Ops.Level,
		m.Compliance.Level,
	}
	max := 0
	for _, level := range levels {
		if level > max {
			max = level
		}
	}
	return max
}

func NormalizeRiskMatrix(m RiskMatrix) RiskMatrix {
	m.Security = normalizeRiskDimension(m.Security)
	m.DataIntegrity = normalizeRiskDimension(m.DataIntegrity)
	m.Compatibility = normalizeRiskDimension(m.Compatibility)
	m.Performance = normalizeRiskDimension(m.Performance)
	m.Ops = normalizeRiskDimension(m.Ops)
	m.Compliance = normalizeRiskDimension(m.Compliance)
	return m
}

func normalizeRiskDimension(d RiskDimension) RiskDimension {
	if d.Level < 0 {
		d.Level = 0
	}
	if d.Level > 5 {
		d.Level = 5
	}
	return d
}
