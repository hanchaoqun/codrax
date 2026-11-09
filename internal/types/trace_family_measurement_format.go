package types

import (
	"fmt"
	"math"
	"strings"
)

// Native interval measures shared by the producer and answer surfaces.
const (
	TraceFamilyFoldSumDisjoint   = "sum_disjoint"
	TraceFamilyFoldIntervalUnion = "interval_union"
)

// TraceSemanticWorkLabel is the reader label for a typed semantic class.
// Callers can keep a generic label for a class they do not recognize.
func TraceSemanticWorkLabel(class string, zh bool) string {
	var en, cn string
	switch class {
	case "class_verification":
		en, cn = "class verification", "类校验"
	case "jit_compile":
		en, cn = "JIT compilation", "JIT编译"
	case "shader_compile":
		en, cn = "shader compilation", "着色器编译"
	case "runtime_compile":
		en, cn = "runtime compilation", "运行时编译"
	case "texture_upload":
		en, cn = "texture upload", "纹理上传"
	case "gc_pause":
		en, cn = "GC pause", "GC暂停"
	}
	if zh {
		return cn
	}
	return en
}

// FormatTraceFamilyMeasurement keeps an engine family value beside its own
// record population and folding ruler on answer-writing surfaces. It consumes
// FamilyMember* only: display-fold Merged* counts and another query's physical
// occurrence inventory are different populations. Missing measurements remain
// absent; this formatter creates no fact, relation or ranking authority.
func FormatTraceFamilyMeasurement(count int, maximum float64, fold, lang string) string {
	if count <= 1 {
		return ""
	}
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	var parts []string
	if zh {
		parts = append(parts, fmt.Sprintf("%d 条计量记录（不是物理发生次数）", count))
	} else {
		parts = append(parts, fmt.Sprintf("%d measurement records (not physical occurrences)", count))
	}
	var ruler string
	timed := false
	switch strings.TrimSpace(fold) {
	case TraceFamilyFoldSumDisjoint:
		ruler, timed = "sum of disjoint intervals within this family", true
		if zh {
			ruler = "本组互斥区间求和"
		}
	case TraceFamilyFoldIntervalUnion:
		ruler, timed = "interval union within this family", true
		if zh {
			ruler = "本组区间去重并集"
		}
	case "max_overlap_fallback":
		ruler = "maximum-record fallback; overlap not resolved"
		if zh {
			ruler = "取记录最大值回退；重叠尚未消解"
		}
	case "count_sum":
		ruler = "count-equivalent aggregation, not wall-clock time"
		if zh {
			ruler = "计数当量聚合，不是墙钟时长"
		}
	default:
		ruler = "aggregation basis not established"
		if zh {
			ruler = "聚合口径未明确"
		}
	}
	// Preserve every positive finite maximum, but only an explicitly timed
	// fold licenses milliseconds. The fallback also admits non-interval
	// composite/advisory measures; count-equivalent and unknown rulers must
	// not acquire a guessed duration unit. Zero is the absent wire value.
	if maximum > 0 && !math.IsNaN(maximum) && !math.IsInf(maximum, 0) {
		if zh && timed {
			parts = append(parts, fmt.Sprintf("记录最大值 %.3f 毫秒（不一定是单次时长）", maximum))
		} else if timed {
			parts = append(parts, fmt.Sprintf("record maximum %.3f ms (not necessarily one occurrence)", maximum))
		} else if zh {
			parts = append(parts, fmt.Sprintf("记录最大值 %.3f（沿用本行计量口径，非单次时长）", maximum))
		} else {
			parts = append(parts, fmt.Sprintf("record maximum %.3f (in this row's measure, not a single-occurrence duration)", maximum))
		}
	}
	parts = append(parts, ruler)
	if zh {
		return strings.Join(parts, "；")
	}
	return strings.Join(parts, "; ")
}
