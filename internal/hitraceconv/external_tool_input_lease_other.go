//go:build !linux

package hitraceconv

import (
	"os"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func tryExternalToolInheritedInputPlatform(
	conversionInputView,
	externalToolInputProfile,
) (*os.File, filegeneration.Identity, bool, error) {
	return nil, filegeneration.Identity{}, false, nil
}
