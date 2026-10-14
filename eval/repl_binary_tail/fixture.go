package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const previewBytes = 1024
const question = "这份采集在 1.0105 到 1.012 秒唤醒了哪个线程？请列出时间、线程名称和线程编号。"

// Same production RMQ wire format as cmd.cliBinaryTraceTailFixture and
// repl.hmc17BinaryTraceTailFixture. Constructed data, not a device recording.
func rmqFixture() []byte {
	var out bytes.Buffer
	header := make([]byte, 12)
	binary.LittleEndian.PutUint16(header[0:2], 0x0ace)
	header[2] = 1
	binary.LittleEndian.PutUint16(header[4:6], 1)
	binary.LittleEndian.PutUint32(header[8:12], 2)
	out.Write(header)
	segment := func(kind uint32, data []byte) {
		var hdr [8]byte
		binary.LittleEndian.PutUint32(hdr[:4], kind)
		binary.LittleEndian.PutUint32(hdr[4:], uint32(len(data)))
		out.Write(hdr[:])
		out.Write(data)
	}
	format := "name: sched_wakeup\nID: 10\nformat:\n" +
		"\tfield:unsigned short common_type;\toffset:0;\tsize:2;\tsigned:0;\n" +
		"\tfield:unsigned char common_flags;\toffset:2;\tsize:1;\tsigned:0;\n" +
		"\tfield:unsigned char common_preempt_count;\toffset:3;\tsize:1;\tsigned:0;\n" +
		"\tfield:int common_pid;\toffset:4;\tsize:4;\tsigned:1;\n" +
		"\tfield:char comm[16];\toffset:8;\tsize:16;\tsigned:0;\n" +
		"\tfield:int pid;\toffset:24;\tsize:4;\tsigned:1;\n" +
		"\tfield:int prio;\toffset:28;\tsize:4;\tsigned:1;\n" +
		"\tfield:int target_cpu;\toffset:32;\tsize:4;\tsigned:1;\n" +
		"print fmt: \"comm=%s pid=%d prio=%d target_cpu=%03d\"\n"
	segment(1, []byte(format))
	segment(2, []byte("12 worker\n"))
	segment(3, []byte("12 12\n"))
	var pages bytes.Buffer
	for i := 0; i < 12; i++ {
		page := make([]byte, 4096)
		binary.LittleEndian.PutUint64(page[0:8], uint64(1_000_000_000+i*1_000_000))
		binary.LittleEndian.PutUint64(page[8:16], 42)
		binary.LittleEndian.PutUint16(page[21:23], 36)
		payload := page[23:59]
		binary.LittleEndian.PutUint16(payload[:2], 10)
		binary.LittleEndian.PutUint32(payload[4:8], 12)
		name, pid := "head-worker", uint32(42)
		if i == 11 {
			name, pid = "tail-target", 424242
		}
		copy(payload[8:24], name)
		binary.LittleEndian.PutUint32(payload[24:28], pid)
		binary.LittleEndian.PutUint32(payload[28:32], 120)
		pages.Write(page)
	}
	segment(4, pages.Bytes())
	return out.Bytes()
}

type fixtureOracle struct {
	Kind          string  `json:"kind"`
	InputSHA256   string  `json:"input_sha256"`
	PreviewLimit  int     `json:"preview_limit"`
	PreviewBytes  int     `json:"actual_preview_bytes"`
	PreviewSHA256 string  `json:"preview_sha256"`
	TailInPreview bool    `json:"tail_in_preview"`
	FullEvents    int     `json:"full_events"`
	TimeStart     float64 `json:"time_start"`
	TimeEnd       float64 `json:"time_end"`
	ExpectedTime  float64 `json:"expected_time"`
	ExpectedName  string  `json:"expected_name"`
	ExpectedPID   int     `json:"expected_pid"`
}

// Oracle preparation is isolated from the child workspace. Neither its
// converted files nor its preview/result/receipt are supplied to the CLI.
func prepareOracle(ctx context.Context, input, out string) (fixtureOracle, error) {
	material, err := traceinput.Prepare(ctx, traceinput.Options{InputPath: input, RuntimeAnchor: out, PreviewBytes: previewBytes})
	if err != nil {
		return fixtureOracle{}, err
	}
	preview := material.Preview()
	idx, err := tracequery.BuildIndex(ctx, material.QueryPath())
	if err != nil {
		return fixtureOracle{}, err
	}
	rows := tracequery.Run(idx, tracequery.Query{View: "event_search", TimeStart: 1.0105, TimeEnd: 1.012, Limit: 20})
	if material.QueryPath() == input || material.SelfContainedText() || len(preview) > previewBytes || strings.Contains(preview, "tail-target") || strings.Contains(preview, "424242") || !strings.Contains(preview, "truncated") || len(idx.Events) != 12 || len(rows.Events) != 1 {
		return fixtureOracle{}, fmt.Errorf("fixture lost binary/complete-tail/preview separation")
	}
	last := idx.Events[len(idx.Events)-1]
	if last.Type != tracequery.EventSchedWakeup || last.Ts != 1.011 || last.WakeePID != 424242 || last.WakeeComm != "tail-target" {
		return fixtureOracle{}, fmt.Errorf("unexpected fixture tail: %+v", last)
	}
	if err := os.WriteFile(filepath.Join(out, "preview.txt"), []byte(preview), 0600); err != nil {
		return fixtureOracle{}, err
	}
	if err := writeJSON(filepath.Join(out, "query.json"), rows); err != nil {
		return fixtureOracle{}, err
	}
	sha, err := fileSHA(input)
	return fixtureOracle{Kind: "constructed_rmq_not_device_capture", InputSHA256: sha, PreviewLimit: previewBytes, PreviewBytes: len(preview), PreviewSHA256: bytesSHA([]byte(preview)), FullEvents: len(idx.Events), TimeStart: 1.0105, TimeEnd: 1.012, ExpectedTime: last.Ts, ExpectedName: last.WakeeComm, ExpectedPID: last.WakeePID}, err
}
