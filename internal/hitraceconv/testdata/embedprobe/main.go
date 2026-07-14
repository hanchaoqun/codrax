package main

import (
	"encoding/json"
	"os"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

func main() {
	status, err := hitraceconv.BuildTraceToolStatus(hitraceconv.Options{})
	if err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(status.TraceStreamer); err != nil {
		panic(err)
	}
}
