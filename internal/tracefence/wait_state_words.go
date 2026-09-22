package tracefence

import "strings"

// WaitStateWithOriginal keeps the recorded scheduler state separate from the
// accounting category. The caller supplies native optional evidence, never a
// state inferred from the category; old records retain their existing label.
func WaitStateWithOriginal(category, original string, zh bool) string {
	original = strings.TrimSpace(original)
	if original == "" {
		return category
	}
	if zh {
		return "原始调度状态：" + original + "；统计类别：" + category
	}
	return "original scheduler state: " + original + "; accounting category: " + category
}
