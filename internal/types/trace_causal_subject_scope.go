package types

import (
	"fmt"
	"strings"
)

// TraceThreadCoordinate is the native thread record on one query board. Comm
// is a display label, not a thread key or proof of process ownership.
type TraceThreadCoordinate struct {
	TID  int    `json:"tid"`
	TGID int    `json:"tgid,omitempty"`
	Comm string `json:"comm,omitempty"`
}

func (c TraceThreadCoordinate) valid() bool {
	return c.TID > 0 && c.TID <= RuntimeTargetMaxPID && c.TGID >= 0 && c.TGID <= RuntimeTargetMaxPID
}

func (c TraceThreadCoordinate) withoutUnknownTGID() TraceThreadCoordinate {
	if c.TGID < 0 || c.TGID > RuntimeTargetMaxPID {
		c.TGID = 0
	}
	return c
}

// TraceRankSubjects rides only the in-process native observation. Restore or
// model JSON cannot supply it. The board target and the ranked member are
// deliberately separate, including when a cross-process member is on-chain.
type TraceRankSubjects struct {
	Target TraceThreadCoordinate `json:"query_target"`
	Member TraceThreadCoordinate `json:"member"`
}

type TraceRankRequestBinding string

const (
	TraceRankRequestUnbound     TraceRankRequestBinding = "unbound"
	TraceRankRequestedThreadID  TraceRankRequestBinding = "requested_thread_id"
	TraceRankRequestedProcessID TraceRankRequestBinding = "requested_process_id"
)

// TraceRankSubjectScope is a disclosure of the board's relationship to the
// requested subject, independent of time scope, causality and completeness.
// A match is to an explicit number in the typed request, not to a name or a
// complete application population; it is never an admission or ranking input.
type TraceRankSubjectScope struct {
	TraceRankSubjects
	Binding TraceRankRequestBinding `json:"request_binding"`
}

const TraceRankSubjectScopeTeaching = "A matching time window does not bind an application. `query_subject_scope` separates the selected thread, the ranked member and the requested subject. `unbound` limits causal conclusions to the queried chain; independent system measurements still stand at their own scope. Within this board, equal recorded TID and TGID under different comm labels do not show two threads. A matched requested ID scopes only the target; a system total does not establish application ownership. This is evidence context, not a tool argument."

type traceRankUserFocus struct {
	// IDs are collected only from the validated request lane. Name-only
	// targets remain useful for existing discovery and election, but names
	// cannot independently establish app ownership in this handoff.
	threads, processes map[int]bool
}

func traceRankUserFocusFromRequest(rm *RequestModel) *traceRankUserFocus {
	if rm == nil || rm.RuntimeTargetProfile == nil || !rm.RuntimeTargetProfile.Declaration.IsValid() ||
		rm.RuntimeTargetProfile.Declaration == RuntimeTargetDeclarationNotApplicable {
		return nil
	}
	focus := &traceRankUserFocus{}
	if !rm.RuntimeTargetProfile.NamedTarget() {
		return focus
	}
	for _, target := range rm.RuntimeTargets {
		if strings.TrimSpace(target.Source) != "user_explicit" || target.PID <= 0 || target.PID > RuntimeTargetMaxPID {
			continue
		}
		switch NormalizeRuntimeTargetKind(target.Kind) {
		case RuntimeTargetKindThread:
			if focus.threads == nil {
				focus.threads = make(map[int]bool)
			}
			focus.threads[target.PID] = true
		case RuntimeTargetKindProcess:
			if focus.processes == nil {
				focus.processes = make(map[int]bool)
			}
			focus.processes[target.PID] = true
		}
	}
	return focus
}

func (p TraceCausalProjection) RankSubjectScope(n TraceCausalProjectionNode) *TraceRankSubjectScope {
	if p.rankUserFocus == nil || n.RankSubjects == nil {
		return nil
	}
	s := &TraceRankSubjectScope{TraceRankSubjects: TraceRankSubjects{
		Target: n.RankSubjects.Target.withoutUnknownTGID(), Member: n.RankSubjects.Member.withoutUnknownTGID(),
	}, Binding: TraceRankRequestUnbound}
	if !s.Target.valid() || !s.Member.valid() {
		return nil
	}
	if p.rankUserFocus.threads[s.Target.TID] {
		s.Binding = TraceRankRequestedThreadID
	} else if p.rankUserFocus.processes[s.Target.TGID] {
		s.Binding = TraceRankRequestedProcessID
	}
	return s
}

func CloneTraceRankSubjectScope(s *TraceRankSubjectScope) *TraceRankSubjectScope {
	if s == nil || !s.Target.valid() || !s.Member.valid() {
		return nil
	}
	switch s.Binding {
	case TraceRankRequestUnbound, TraceRankRequestedThreadID, TraceRankRequestedProcessID:
		copy := *s
		return &copy
	default:
		return nil
	}
}

// Format is shared by the report header and any reader of the bound scope.
// A repeated number is not a thread-lifetime or same-process proof across
// other queries or captures; this message describes only the selected board.
func (s TraceRankSubjectScope) Format(zh bool) string {
	target := s.Target
	coordinate := fmt.Sprintf("TID %d", target.TID)
	if target.TGID > 0 {
		coordinate += fmt.Sprintf(" / TGID %d", target.TGID)
	}
	if zh {
		if s.Binding == TraceRankRequestUnbound {
			return "这条因果链围绕查询选中的线程 " + coordinate + "；这条链的证据不能外推为应用层面的结论。"
		}
		return "这条因果链围绕查询选中的线程 " + coordinate + "；其线程或进程编号与请求中的编号相符，不代表整应用或所有链上成员。"
	}
	if s.Binding == TraceRankRequestUnbound {
		return "This chain is for the query-selected thread " + coordinate + "; the evidence for this chain does not establish an application-wide conclusion."
	}
	return "This chain is for the query-selected thread " + coordinate + "; its thread or process ID matches the requested ID, not the entire application or every member of the chain."
}
