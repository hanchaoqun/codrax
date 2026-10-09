package tracequery

import "strings"

// These signatures are navigation clues adapted from the five reference
// pipeline definitions, not their score/required/exclusion decision machine.
// Shared Draw/FlushBuffer names alone cannot select a framework. Distinct
// frameworks and composition layers may coexist in the same observed owner.
type renderingCandidateRule struct {
	framework, kind, pattern, role string
	unsupported                    bool
}

var renderingCandidateRules = []renderingCandidateRule{
	{"HARMONY_ARKUI", "slice_name", "*OnVsyncCallback*", "UI callback candidate", false},
	{"HARMONY_ARKUI", "slice_name", "*ReceiveVsync*", "UI callback candidate", false},
	{"HARMONY_ARKUI", "slice_name", "*OnVsyncEvent*", "UI callback candidate", false},
	{"HARMONY_ARKUI", "slice_name", "*FlushMessages*", "UI submission candidate", false},
	{"HARMONY_ARKUI", "slice_name", "*SendCommands*", "UI submission candidate", false},
	{"HARMONY_ARKUI", "slice_name", "*MarshRSTransactionData*", "UI submission candidate", false},
	{"HARMONY_ARKUI", "slice_name", "*UIVsyncTask*", "UI callback candidate", false},
	{"HARMONY_ARKUI", "thread_name", "ArkUIEngine*", "UI engine candidate", false},
	{"HARMONY_KMP", "slice_name", "*Recomposer*", "composition candidate", false},
	{"HARMONY_KMP", "slice_name", "*RenderView*onDraw*", "drawing candidate", false},
	{"HARMONY_KMP", "slice_name", "*ffi:*", "native interop candidate", false},
	{"HARMONY_RN", "slice_name", "*RNView*", "native UI candidate", false},
	{"HARMONY_RN", "slice_name", "*CRNNode*", "native UI candidate", false},
	{"HARMONY_RN", "slice_name", "*RNNode*", "native UI candidate", false},
	{"HARMONY_RN", "thread_name", "mqt_js", "JavaScript thread candidate", false},
	{"HARMONY_FLUTTER", "slice_name", "*flutter::*", "unknown", false},
	{"HARMONY_FLUTTER", "slice_name", "*VsyncWaiter*", "VSync callback candidate", false},
	{"HARMONY_FLUTTER", "slice_name", "*Impeller*", "renderer candidate", false},
	{"HARMONY_FLUTTER", "thread_name", "*.ui", "Dart UI thread candidate", false},
	{"HARMONY_FLUTTER", "thread_name", "*.raster", "raster thread candidate", false},
	{"HARMONY_FLUTTER", "thread_name", "*.io", "IO thread candidate", false},
	{"HARMONY_WEB_PIPELINE", "slice_name", "*ExternalBeginFrameSource*", "frame callback candidate", false},
	{"HARMONY_WEB_PIPELINE", "slice_name", "*Graphics.Pipeline*IssueBeginFrame*", "frame callback candidate", false},
	{"HARMONY_WEB_PIPELINE", "slice_name", "*DisplayScheduler::*", "display scheduling candidate", false},
	{"HARMONY_WEB_PIPELINE", "slice_name", "*RasterizerTaskImpl*", "raster task candidate", false},
	{"HARMONY_WEB_PIPELINE", "slice_name", "*SendBeginFrameDecision*", "frame callback candidate", false},
	{"HARMONY_WEB_PIPELINE", "slice_name", "*VideoFrameSubmitter*", "video submit candidate", false},
	{"HARMONY_WEB_PIPELINE", "thread_name", "VizCompositor*", "compositor thread candidate", false},
	{"HARMONY_WEB_PIPELINE", "thread_name", "CompositorGpuTh*", "compositor GPU thread candidate", false},
	{"HARMONY_WEBVIEW_GL_FUNCTOR", "slice_name", "*DrawGLFunctor*", "GL functor candidate", true},
	{"HARMONY_WEBVIEW_GL_FUNCTOR", "slice_name", "*DrawFunctor*", "GL functor candidate", true},
	{"HARMONY_RENDER_SERVICE", "thread_name", "render_service", "render service candidate", true},
	{"HARMONY_RENDER_SERVICE", "thread_name", "RSUniRender*", "composition thread candidate", true},
	{"HARMONY_GAME_ENGINE", "slice_name", "*Unity*", "unknown", true},
	{"HARMONY_GAME_ENGINE", "slice_name", "*Unreal*", "unknown", true},
	{"HARMONY_GAME_ENGINE", "slice_name", "*Godot*", "unknown", true},
	{"HARMONY_GAME_ENGINE", "slice_name", "*Cocos*", "unknown", true},
}

// Literal '*' glob only; trace names may contain slash, brackets, pipes or
// arbitrary punctuation. No regex syntax or user/answer prose is evaluated.
func renderingNameMatches(pattern, value string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == value
	}
	if !strings.HasPrefix(value, parts[0]) {
		return false
	}
	value = value[len(parts[0]):]
	for _, part := range parts[1 : len(parts)-1] {
		i := strings.Index(value, part)
		if i < 0 {
			return false
		}
		value = value[i+len(part):]
	}
	return strings.HasSuffix(value, parts[len(parts)-1])
}
