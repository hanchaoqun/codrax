package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
)

func summarizeCriterionFailures(failed []criterion.Result) string {
	if len(failed) == 0 {
		return "entry conditions not satisfied"
	}
	const maxFailures = 3
	parts := make([]string, 0, min(len(failed), maxFailures))
	for i, f := range failed {
		if i >= maxFailures {
			break
		}
		label := string(f.Kind)
		if f.Expr != "" {
			label += " " + f.Expr
		}
		if f.Detail != "" {
			label += ": " + f.Detail
		}
		parts = append(parts, label)
	}
	if len(failed) > maxFailures {
		parts = append(parts, fmt.Sprintf("+%d more", len(failed)-maxFailures))
	}
	return strings.Join(parts, "; ")
}
