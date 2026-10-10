package types

import (
	"fmt"
	"strings"
)

// logErrorArtifactFact separates an observed error from stack support whose
// error label/message is unknown. The latter never inherits error causality.
type logErrorArtifactFact struct {
	Target, Summary, RawExcerpt string
	FrameOnly                   bool
}

func projectLogErrorArtifactFacts(err LogError) []logErrorArtifactFact {
	if target := firstNonEmptyString(err.ObservedTypeLiteral(), err.Message); target != "" {
		return []logErrorArtifactFact{{Target: target, Summary: firstNonEmptyString(err.Message, err.ObservedTypeLiteral())}}
	}
	var facts []logErrorArtifactFact
	for _, frame := range err.Frames {
		file := strings.TrimSpace(frame.File)
		if file != "" && frame.Line > 0 {
			file = fmt.Sprintf("%s:%d", file, frame.Line)
		}
		target := firstNonEmptyString(frame.Raw, frame.Func, file)
		if target != "" {
			facts = append(facts, logErrorArtifactFact{Target: target, Summary: target, RawExcerpt: strings.TrimSpace(frame.Raw), FrameOnly: true})
		}
	}
	return facts
}
