package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func (s *graphState) setTransientRetryHint(hint string, kind types.RetryHintKind) {
	if s == nil {
		return
	}
	hint = strings.TrimSpace(hint)
	s.transientRetryHint = hint
	if hint == "" {
		s.transientRetryHintKind = types.RetryHintKindUnknown
	} else {
		s.transientRetryHintKind = kind
	}
}

func (s *graphState) consumeTransientRetryHint() (string, types.RetryHintKind) {
	if s == nil {
		return "", types.RetryHintKindUnknown
	}
	hint := strings.TrimSpace(s.transientRetryHint)
	kind := s.transientRetryHintKind
	s.transientRetryHint = ""
	s.transientRetryHintKind = types.RetryHintKindUnknown
	if hint == "" {
		kind = types.RetryHintKindUnknown
	}
	return hint, kind
}
