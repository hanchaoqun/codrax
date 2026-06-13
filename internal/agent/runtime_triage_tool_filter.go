package agent

import (
	"os"
	"path/filepath"
	"strings"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeTriageInlineAttachmentBlocksTool(ctx *types.AgentContext, name string) bool {
	if ctx == nil || types.CanonicalToolName(name) != "read_file" {
		return false
	}
	switch {
	case ctx.Stage == types.StageLogTriage || ctx.AgentName == types.AgentLogTriager:
		return runtimeTriageAttachmentIsInlineOnly(ctx.AttachedLog, ctx.WorkDir, promptctx.AttachedLogBlobName)
	case ctx.Stage == types.StagePerfTriage || ctx.AgentName == types.AgentPerfTriager:
		return runtimeTriageAttachmentIsInlineOnly(ctx.AttachedHitrace, ctx.WorkDir, promptctx.AttachedTraceBlobName)
	default:
		return false
	}
}

func runtimeTriageAttachmentIsInlineOnly(raw, workDir, blobName string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	if strings.TrimSpace(workDir) == "" || strings.TrimSpace(blobName) == "" {
		return true
	}
	_, err := os.Stat(filepath.Join(workDir, blobName))
	return err != nil
}
