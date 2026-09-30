package skill

import (
	"strings"
	"testing"
)

func TestCurrentSourceExplanationTeachingSingleSource(t *testing.T) {
	if got := strings.Count(BuildAnalysisSkill().OutputFormat, AnalysisCurrentSourceExplanationTeaching); got != 1 {
		t.Fatalf("shared secondary-dimension teaching appears %d times", got)
	}
}
