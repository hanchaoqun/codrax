package outputdump

// question_verbatim_test.go — UX-ANCHOR 件d pins (§29.61.7 user ruling,
// 2026-07-14): the 问题 section echoes the customer's own input VERBATIM
// inside a typed text fence — never re-rendered as markdown on any product
// face.
//
//   ① md face: the fence interior is byte-identical to the raw request
//      (control-glyph fixture: #/*/_/```/<tag> mixed);
//   ② fence-collision protection is content-aware (a request carrying an
//      N-backtick run gets an N+1 fence);
//   ③ HTML face: the section falls out as ONE escaped <pre> — headings /
//      emphasis are not re-rendered and <script> never reaches the page as
//      markup (injection-shaped pin).
//
// The classifier-side negative (件d⑤ — the question fence never re-classifies
// as a projection tree or mermaid) lives in internal/preview
// (TestQuestionFenceNeverClassifiesAsTraceOrMermaid), where the classifier is.

import (
	"strings"
	"testing"
)

// questionFixtureRequest is the control-glyph fixture: markdown headings,
// emphasis, an embedded three-backtick fence, raw HTML tags and a script tag.
const questionFixtureRequest = "# 假标题\n" +
	"**加粗** *斜体* _下划_\n" +
	"```go\ncode `tick`\n```\n" +
	"<script>alert(1)</script>\n" +
	"<tag attr=\"x\">尾行无换行"

func TestQuestionSectionVerbatimFenceByteIdentity(t *testing.T) {
	body := BuildBody(Args{Request: questionFixtureRequest, Answer: "ok"})
	// ② the fixture's longest backtick run is 3 (the embedded ```go fence),
	// so the wrapping fence is 4 backticks with the typed info token.
	opener := "````text codrax-user-request\n"
	closer := "\n````\n"
	section := "# 问题\n\n" + opener
	at := strings.Index(body, section)
	if at < 0 {
		t.Fatalf("问题 section did not open a typed verbatim fence:\n%s", body)
	}
	rest := body[at+len(section):]
	end := strings.Index(rest, closer)
	if end < 0 {
		t.Fatalf("verbatim fence is unterminated:\n%s", body)
	}
	// ① byte identity: interior == raw request (+ the single terminating
	// newline the closing fence line requires).
	if got, want := rest[:end+1], questionFixtureRequest+"\n"; got != want {
		t.Fatalf("问题 fence interior is not byte-identical to the request\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestQuestionFenceLengthTracksBacktickRuns(t *testing.T) {
	for _, tc := range []struct {
		request string
		want    string
	}{
		{"plain question", "```text codrax-user-request\n"},
		{"has `` two ticks", "```text codrax-user-request\n"},
		{"has ```` four ticks", "`````text codrax-user-request\n"},
	} {
		body := BuildBody(Args{Request: tc.request, Answer: "ok"})
		if !strings.Contains(body, tc.want+tc.request+"\n") {
			t.Fatalf("request %q: fence opener %q missing:\n%s", tc.request, tc.want, body)
		}
	}
}

func TestQuestionSectionHTMLEscapesInsteadOfRendering(t *testing.T) {
	body := BuildBody(Args{Request: questionFixtureRequest, Answer: "ok"})
	html, err := BuildHTML("t", body)
	if err != nil {
		t.Fatal(err)
	}
	// ③ the question renders as one plain escaped pre.
	if !strings.Contains(html, `<pre><code class="language-text">`) {
		t.Fatalf("问题 fence must render as a plain escaped pre:\n%s", html)
	}
	// Injection-shaped pin: the script tag reaches the page ESCAPED only.
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatalf("customer <script> reached the page as markup:\n%s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("customer <script> must stay visible as escaped text:\n%s", html)
	}
	// Markdown control glyphs are NOT re-rendered.
	if strings.Contains(html, ">假标题</h1>") || strings.Contains(html, "<strong>加粗</strong>") {
		t.Fatalf("customer markdown was re-rendered inside the 问题 section:\n%s", html)
	}
	if !strings.Contains(html, "# 假标题") || !strings.Contains(html, "**加粗** *斜体* _下划_") {
		t.Fatalf("customer markdown must stay verbatim text:\n%s", html)
	}
}

func TestQuestionSectionEmptyRequestKeepsLabel(t *testing.T) {
	body := BuildBody(Args{Request: "", Answer: "ok"})
	if !strings.Contains(body, "# 问题\n\n(空)\n") {
		t.Fatalf("empty request must keep the plain label, not a fence:\n%s", body)
	}
	if strings.Contains(body, "codrax-user-request") {
		t.Fatalf("empty request must not mint a verbatim fence:\n%s", body)
	}
}
