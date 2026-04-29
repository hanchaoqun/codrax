package tool

import (
	"fmt"
	"strconv"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func normalizeSegmentationRange(start, end, total int) (int, int, bool, bool) {
	if total <= 0 {
		return 0, 0, false, false
	}
	origStart, origEnd := start, end
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	if end < 0 {
		end = 0
	}
	if end > total {
		end = total
	}
	if end <= start {
		return 0, 0, start != origStart || end != origEnd, false
	}
	return start, end, start != origStart || end != origEnd, true
}

func segmentationCoordinateRejected(toolName, artifactLabel string, total, attempted int) types.ToolResult {
	if total <= 0 {
		return types.ToolResult{
			ToolName:  toolName,
			Success:   false,
			Summary:   fmt.Sprintf("%s rejected: the attached %s body is empty in this dispatch, so byte ranges cannot be validated. Segment only the raw attached %s body, not repository files or prompt prose.", toolName, artifactLabel, artifactLabel),
			Repair:    segmentationCoordinateRepair(toolName, artifactLabel, total),
			Timestamp: time.Now(),
		}
	}
	return types.ToolResult{
		ToolName: toolName,
		Success:  false,
		Summary: fmt.Sprintf("%s rejected: no valid segments after coordinate validation (attachment_bytes=%d, attempted=%d). Coordinates must satisfy 0 <= byte_start < byte_end <= %d and be measured over the raw attached %s body only, not over prompt prose / code fences / file previews.",
			toolName, total, attempted, total, artifactLabel),
		Repair:    segmentationCoordinateRepair(toolName, artifactLabel, total),
		Timestamp: time.Now(),
	}
}

func segmentationCoordinateRepair(toolName, artifactLabel string, total int) *types.ToolRepair {
	return &types.ToolRepair{
		Code:   "segmentation_coordinates",
		Hint:   fmt.Sprintf("Re-emit `%s` with byte ranges measured over the raw attached %s body only. Keep every segment inside `[0, %d]`, and remember that `byte_end` is exclusive. If only a few blocks matter, emit just the essential stack / trace regions instead of over-segmenting.", toolName, artifactLabel, total),
		Fields: []string{"segments"},
		Metadata: map[string]string{
			"attachment_bytes": strconv.Itoa(total),
			"artifact":         artifactLabel,
		},
	}
}
