package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHMC221InputRoutingProbeContentAndSchemaNotSuffix(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ddl   []string
		views []string
	}{
		{"native_empty", []string{"CREATE TABLE measure(ts,value,filter_id)", "CREATE TABLE measure_filter(id)"}, []string{"measurements"}},
		{"business", []string{"CREATE TABLE orders(ts,value,customer_id)"}, nil},
		{"partial", []string{"CREATE TABLE measure(ts,value)", "CREATE TABLE measure_filter(id)"}, nil},
		{"cpu", []string{"CREATE TABLE measure(ts,value,filter_id)", "CREATE TABLE cpu_measure_filter(id,name,cpu)"}, []string{"cpu_state_frequency"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := createTraceDBFixture(t, tc.ddl)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"unknown", "capture.data", "misleading.csv"} {
				input := filepath.Join(t.TempDir(), name)
				if err := os.WriteFile(input, before, 0400); err != nil {
					t.Fatal(err)
				}
				anchor := t.TempDir()
				got := ProbeInputRoutingCapability(context.Background(), input, anchor)
				if got.Container != "sqlite" || got.Status != "schema_inspected" || !reflect.DeepEqual(got.CandidateViews, tc.views) {
					t.Fatalf("probe %s: %+v", name, got)
				}
				if (got.NativeReader == "trace_query") != (len(tc.views) > 0) {
					t.Fatalf("SQLite magic alone minted native capability: %+v", got)
				}
				after, err := os.ReadFile(input)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("probe changed source", err)
				}
				entries, err := os.ReadDir(filepath.Dir(input))
				if err != nil || len(entries) != 1 {
					t.Fatal("source-side files changed", entries, err)
				}
				entries, err = os.ReadDir(anchor)
				if err != nil || len(entries) != 0 {
					t.Fatal("private probe staging not cleaned", entries, err)
				}
			}
		})
	}
}

func TestHMC221InputRoutingProbeUnknownBoundaries(t *testing.T) {
	for _, boundary := range []string{"cancelled", "wal", "auxiliary", "oversize", "malformed", "container"} {
		t.Run(boundary, func(t *testing.T) {
			path := createTraceDBFixture(t, []string{"CREATE TABLE measure(ts,value,filter_id)", "CREATE TABLE measure_filter(id)"})
			ctx := context.Background()
			switch boundary {
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "wal":
				body, _ := os.ReadFile(path)
				body[18], body[19] = 2, 2
				if err := os.WriteFile(path, body, 0600); err != nil {
					t.Fatal(err)
				}
			case "auxiliary":
				if err := os.WriteFile(path+"-wal", []byte("active"), 0600); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				if err := os.Truncate(path, InputRoutingSQLiteMaxBytes+1); err != nil {
					t.Fatal(err)
				}
			case "malformed":
				if err := os.WriteFile(path, []byte("SQLite format 3\x00broken"), 0600); err != nil {
					t.Fatal(err)
				}
			case "container":
				if err := os.WriteFile(path, []byte{'P', 'K', 3, 4}, 0600); err != nil {
					t.Fatal(err)
				}
			}
			anchor := t.TempDir()
			got := ProbeInputRoutingCapability(ctx, path, anchor)
			if got.NativeReader != "" || len(got.CandidateViews) != 0 || got.Status != "unknown" {
				t.Fatalf("unverified input capability upgraded: %+v", got)
			}
			entries, err := os.ReadDir(anchor)
			if err != nil || len(entries) != 0 {
				t.Fatal("unexpected staging retained", entries, err)
			}
		})
	}
}

// Exercise a physical same-byte replacement after the public probe's first
// context check, not a synthetic metadata claim or a resurrected cached result.
type routingProbeBoundaryContext struct {
	context.Context
	checks  int
	replace func()
}

func (c *routingProbeBoundaryContext) Err() error {
	c.checks++
	if c.checks == 2 {
		c.replace()
	}
	return c.Context.Err()
}

func TestHMC221InputRoutingProbeRejectsChangedGeneration(t *testing.T) {
	path := createTraceDBFixture(t, []string{"CREATE TABLE measure(ts,value,filter_id)", "CREATE TABLE measure_filter(id)"})
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &routingProbeBoundaryContext{Context: context.Background(), replace: func() {
		replacement := path + ".replacement"
		if err := os.WriteFile(replacement, body, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, path); err != nil {
			t.Fatal(err)
		}
	}}
	got := ProbeInputRoutingCapability(ctx, path, t.TempDir())
	if ctx.checks < 2 || got.Status != "unknown" || got.NativeReader != "" || len(got.CandidateViews) != 0 {
		t.Fatalf("changed generation minted capability: %+v (checks=%d)", got, ctx.checks)
	}
}
