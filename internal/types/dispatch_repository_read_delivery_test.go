package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func deliveryReadFixture(t *testing.T, m *MutableState, digest, raw, summary string, start, end, total int) ToolResult {
	t.Helper()
	const root, path = "/physical/repo", "tests/check.py"
	m.RecordDispatchRepositoryFileRead(root, path, raw)
	if !m.RecordDispatchRepositoryFileReadVersionWithSummary(m.BeginDispatchRepositoryFileRead(), root, path, raw, digest, start, end, total, summary) {
		t.Fatal("producer version was not recorded")
	}
	r := ToolResult{ToolName: "read_file", Success: true, Summary: summary, RawRef: raw,
		ReadCoverage: &ToolReadCoverage{Path: path, RawRef: raw, LineStart: start, LineEnd: end, TotalLines: total}}
	m.AppendDispatchToolResult(r)
	return r
}

func TestDispatchRepositoryDeliveredReadQualification(t *testing.T) {
	for _, kind := range []string{"exact", "advisory_tail", "changed_body", "copied_ref", "wrong_id", "json_ticket", "other_state", "reset", "legacy_version"} {
		t.Run(kind, func(t *testing.T) {
			m := NewMutableState("read delivery")
			digest := strings.Repeat("a", 64)
			r := deliveryReadFixture(t, m, digest, "blob-1", "[tests/check.py]\n1: assert value == 7\n", 1, 1, 1)
			generation := m.BeginDispatchRepositoryFileRead()
			if !m.CompleteDispatchRepositoryFileReadVersion("/physical/repo", "tests/check.py", digest) || m.CompleteDispatchDeliveredRepositoryFileReadVersion("/physical/repo", "tests/check.py", digest) {
				t.Fatal("read append and actual delivery were conflated")
			}
			if kind == "legacy_version" {
				m.ResetDispatchToolResults()
				generation = m.BeginDispatchRepositoryFileRead()
				m.RecordDispatchRepositoryFileRead("/physical/repo", "tests/check.py", r.RawRef)
				m.RecordDispatchRepositoryFileReadVersion(generation, "/physical/repo", "tests/check.py", r.RawRef, digest, 1, 1, 1)
				m.AppendDispatchToolResult(r)
			}
			if kind == "advisory_tail" {
				r.Summary = strings.TrimRight(r.Summary, "\n") + "\n[typed navigation note]"
				m.AppendDispatchToolResult(r)
			}
			if kind == "changed_body" || kind == "copied_ref" {
				r.Summary = strings.ReplaceAll(r.Summary, "7", "8")
				if kind == "copied_ref" { // even appended copied identities cannot supply the producer bytes
					m.AppendDispatchToolResult(r)
				}
			}
			ticket := m.BindDispatchRepositoryReadMessage(generation, "read-1", r)
			if kind == "json_ticket" {
				data, _ := json.Marshal(ticket)
				if string(data) != "{}" {
					t.Fatalf("private receipt escaped JSON: %s", data)
				}
				ticket = DispatchRepositoryReadMessageReceipt{}
				if err := json.Unmarshal(data, &ticket); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "wrong_id" {
				if ticket.MatchesMessage("another-read", r.Summary) {
					t.Fatal("transport identity not checked")
				}
				return
			}
			if kind == "other_state" {
				if NewMutableState("other").RecordDispatchRepositoryReadDelivery(generation, []DispatchRepositoryReadMessageReceipt{ticket}) {
					t.Fatal("another state borrowed receipt")
				}
				return
			}
			if kind == "reset" {
				m.ResetDispatchToolResults()
				m.AppendDispatchToolResult(r)
			}
			want := kind == "exact" || kind == "advisory_tail"
			if got := m.RecordDispatchRepositoryReadDelivery(generation, []DispatchRepositoryReadMessageReceipt{ticket}); got != want {
				t.Fatalf("delivery commit=%v, want %v", got, want)
			}
			if got := m.CompleteDispatchDeliveredRepositoryFileReadVersion("/physical/repo", "tests/check.py", digest); got != want {
				t.Fatalf("complete delivered=%v, want %v", got, want)
			}
			if m.CompleteDispatchDeliveredRepositoryFileReadVersion("/other", "tests/check.py", digest) || m.CompleteDispatchDeliveredRepositoryFileReadVersion("/physical/repo", "tests/check.py", strings.Repeat("b", 64)) {
				t.Fatal("root/version qualification was lost")
			}
		})
	}
}

func TestDispatchRepositoryDeliveredReadRequiresGapFreeDeliveredPages(t *testing.T) {
	m := NewMutableState("pages")
	digest := strings.Repeat("a", 64)
	var tickets []DispatchRepositoryReadMessageReceipt
	for i, page := range []struct {
		raw, summary string
		start, end   int
	}{{"head", "1: first\n", 1, 1}, {"tail", "3: last\n", 3, 3}, {"middle", "2: second\n", 2, 2}} {
		r := deliveryReadFixture(t, m, digest, page.raw, page.summary, page.start, page.end, 3)
		ticket := m.BindDispatchRepositoryReadMessage(m.BeginDispatchRepositoryFileRead(), page.raw, r)
		if !ticket.MatchesMessage(page.raw, r.Summary) {
			t.Fatal("page message did not bind")
		}
		tickets = append(tickets, ticket)
		if i < 2 && !m.RecordDispatchRepositoryReadDelivery(m.BeginDispatchRepositoryFileRead(), []DispatchRepositoryReadMessageReceipt{ticket}) {
			t.Fatal("page delivery failed")
		}
	}
	if !m.CompleteDispatchRepositoryFileReadVersion("/physical/repo", "tests/check.py", digest) || m.CompleteDispatchDeliveredRepositoryFileReadVersion("/physical/repo", "tests/check.py", digest) {
		t.Fatal("appended but undelivered middle filled delivery gap")
	}
	if m.RecordDispatchRepositoryReadDelivery(m.BeginDispatchRepositoryFileRead(), []DispatchRepositoryReadMessageReceipt{tickets[2], {}}) || m.CompleteDispatchDeliveredRepositoryFileReadVersion("/physical/repo", "tests/check.py", digest) {
		t.Fatal("failed request snapshot partially committed")
	}
	if !m.RecordDispatchRepositoryReadDelivery(m.BeginDispatchRepositoryFileRead(), tickets[2:]) || !m.CompleteDispatchDeliveredRepositoryFileReadVersion("/physical/repo", "tests/check.py", digest) {
		t.Fatal("delivered middle did not close exact-version coverage")
	}
}
