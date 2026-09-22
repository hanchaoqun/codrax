package agent

import "fmt"

// Keep the analyzer's business label separate from its presentation index.
// Callers retain their existing label fallback and ordering policies.
func requestedAnswerDimensionDisplayInstruction(zh bool) string {
	if zh {
		return "“用户可见标签”后的文字才是成文标签；“内部排序”只用于安排输出顺序，不要写进答案，也不要追加系统内部角色或枚举名。\n"
	}
	return "The text after 'User-facing label' is the visible label. 'Internal order' only arranges the output sequence; do not include it in the answer or append internal system roles or enum names.\n"
}

func requestedAnswerDimensionDisplayRow(index int, label string, zh bool) string {
	if zh {
		return fmt.Sprintf("- 用户可见标签：%s\n  内部排序：%d", label, index)
	}
	return fmt.Sprintf("- User-facing label: %s\n  Internal order: %d", label, index)
}
