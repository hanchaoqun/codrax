package tool

import (
	"path"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryUniverseAppendSupportRefKeys(target map[string]bool, ref string) map[string]bool {
	if target == nil {
		target = map[string]bool{}
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return target
	}
	add := func(raw string) {
		if key := sourceInventoryUniverseSurfaceKey(raw); key != "" {
			target[key] = true
		}
		normalized := strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`), "/")
		if normalized != "" && strings.Contains(normalized, "/") {
			if key := sourceInventoryUniverseSurfaceKey(path.Base(normalized)); key != "" {
				target[key] = true
			}
		}
	}
	add(ref)
	if label, loc, ok := types.ParseAnswerSupportRefMemberLocation(ref); ok {
		add(label)
		add(loc.File)
		return target
	}
	if loc, ok := types.ParseAnswerSourceLocationSurface(ref); ok {
		add(loc.File)
		return target
	}
	if file, ok := types.ParseAnswerFilePathSurface(ref); ok {
		add(file)
	}
	return target
}
