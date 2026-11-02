package tool

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestHMC333PublicResultFormatsDoNotRequireSchemaGuessing(t *testing.T) {
	ctx, published, refs := b1624PublishedResult(t)
	jsonBytes, err := os.ReadFile(refs[1])
	if err != nil {
		t.Fatal(err)
	}
	var result tracequery.Result
	if err := json.Unmarshal(jsonBytes, &result); err != nil || len(result.Events) == 0 {
		t.Fatal("fixture lacks genuine structured native output")
	}
	for name, text := range map[string]string{"published result": published.Summary, "retained report": string(mustReadHMC333(t, refs[0]))} {
		if !strings.Contains(text, "raw_ref is a descriptive text report; payload_ref is the structured JSON result") || !strings.Contains(text, "JSON field names") {
			t.Errorf("%s does not distinguish report headings from JSON field names", name)
		}
	}
	before, _ := json.Marshal(published)
	for _, path := range refs {
		params, _ := json.Marshal(map[string]any{"path": path, "line_offset": 0, "limit": 50})
		read, err := (&ReadFile{}).Execute(ctx, params)
		if err != nil || !read.Success || read.RuntimeArtifactRead == nil || !read.RuntimeArtifactRead.TraceQueryBlob || read.ReadCoverage != nil || len(read.Observations) != 0 {
			t.Fatalf("format guidance changed existing result-read role: %v %+v", err, read)
		}
	}
	after, _ := json.Marshal(published)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(jsonBytes, mustReadHMC333(t, refs[1])) {
		t.Fatal("guidance changed native output or result publication")
	}
}

func mustReadHMC333(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
