package preview

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
)

// §29.61.8a (用户裁定 2026-07-14): the customer-question verbatim fence
// renders as a wrap-enabled pre (long input lines soft-wrap instead of being
// clipped by the box edge). Grid fences (projection tree / ◎ overview) keep
// their no-wrap classes — the wrap class is exclusive to the question fence.
func TestUserRequestFenceRendersWrapEnabledPre(t *testing.T) {
	md := "# 问题\n\n```text " + tracefence.UserRequestInfoToken + "\n" +
		strings.Repeat("很长的客户输入行没有空格也不该被右边框裁掉", 8) + "\n```\n"
	html, err := RenderStandaloneMarkdownHTML("t", []byte(md))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(html, `<pre class="user-request"><code`) {
		t.Fatalf("question fence must render the wrap-enabled pre class:\n%.400s", html)
	}
	if !strings.Contains(html, `pre.user-request { white-space: pre-wrap; overflow-wrap: anywhere; }`) {
		t.Fatalf("page CSS must carry the user-request wrap rule verbatim")
	}
}

func TestProjectionFenceNeverWearsWrapClass(t *testing.T) {
	md := "```text " + tracefence.InfoToken + "\n⊚ demo-1 ‹用户关注线程›\n```\n"
	html, err := RenderStandaloneMarkdownHTML("t", []byte(md))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(html, `trace-projection-tree user-request`) ||
		strings.Contains(html, `<pre class="user-request"><code>⊚`) {
		t.Fatalf("grid fences must keep no-wrap presentation")
	}
	if !strings.Contains(html, `<pre class="trace-projection-tree"`) {
		t.Fatalf("projection fence must keep its grid class")
	}
}
