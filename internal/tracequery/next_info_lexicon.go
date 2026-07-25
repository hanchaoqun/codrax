package tracequery

// next_info_lexicon.go — NEXTINFO P1 (客户 next_info 语义文档, 2026-07-25):
// the single word authority for the next_info closed sets. Every consumer
// face (event detail, constraint policy, future answer surfaces) reads THESE
// tables — never a hand-copied sibling (五表手抄 lesson). Unknown values
// fail open to the "unknown" word with the raw number preserved by callers.

import "fmt"

// NextInfoSchedGroupWord — sched_group closed set (kernel SCHED_LT_* per the
// customer doc): 0=no group, 1=power (small-core pinned, no load accounting),
// 2=energy/mixed, 3=capacity/performance, >=4 unknown extension.
func NextInfoSchedGroupWord(group int, known bool) string {
	if !known {
		return "unknown"
	}
	switch group {
	case 0:
		return "no_group"
	case 1:
		return "power_group"
	case 2:
		return "energy_group"
	case 3:
		return "capacity_group"
	default:
		return fmt.Sprintf("unknown_group_%d", group)
	}
}

// NextInfoSMTExpelWord — smt_expel_type closed set: 0=expellee (1 logical
// core), 1=util expeller (dynamic), 2=static expeller, 3=force, 4=force-long
// (reserved), >=5 unknown.
func NextInfoSMTExpelWord(expel int, known bool) string {
	if !known {
		return "unknown"
	}
	switch expel {
	case 0:
		return "expellee"
	case 1:
		return "util_expeller"
	case 2:
		return "expeller"
	case 3:
		return "force_expeller"
	case 4:
		return "force_expeller_long"
	default:
		return fmt.Sprintf("unknown_expel_%d", expel)
	}
}

// nextInfoSPCGroupNames — cgroup_id SP_* closed set 0-15 (customer doc
// verbatim ordering).
var nextInfoSPCGroupNames = [...]string{
	"SP_DEFAULT",
	"SP_BACKGROUND",
	"SP_FOREGROUND",
	"SP_SYSTEM_BACKGROUND",
	"SP_TOP_APP",
	"SP_BOOST",
	"SP_GRAPHIC",
	"SP_KEY_BACKGROUND",
	"SP_OPT_BACKGROUND",
	"SP_FOREGROUND_APP",
	"SP_NAP_FOREGROUND",
	"SP_NAP_BACKGROUND",
	"SP_SELF_RENDER",
	"SP_GRAPHIC_HIGH",
	"SP_GRAPHIC_NORMAL",
	"SP_LOW_BACKGROUND",
}

// NextInfoSPCGroupName maps cgroup_id to its SP_* name; out-of-set ids keep
// the number visible instead of guessing.
func NextInfoSPCGroupName(cgid int, known bool) string {
	if !known {
		return "unknown"
	}
	if cgid >= 0 && cgid < len(nextInfoSPCGroupNames) {
		return nextInfoSPCGroupNames[cgid]
	}
	return fmt.Sprintf("unknown_cgroup_%d", cgid)
}
