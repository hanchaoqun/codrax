// Diagnostic: dump receiver text distribution from the graph's call
// relations so we can see why unambig_by_receiver_type is small.
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

func main() {
	g, err := repomap.BuildOrLoadGraph(".", "")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Collect type names.
	typeSet := make(map[string]bool)
	for _, fi := range g.Files {
		for i := range fi.Symbols {
			s := &fi.Symbols[i]
			switch s.Kind {
			case "type", "struct", "interface", "class":
				typeSet[s.Name] = true
			}
		}
	}

	total := 0
	withRecv := 0
	recvIsType := 0
	recvHist := make(map[string]int)
	for _, fi := range g.Files {
		for _, rel := range fi.Relations {
			if rel.Kind != "call" {
				continue
			}
			total++
			if rel.ToEP.Receiver == "" {
				continue
			}
			withRecv++
			if typeSet[rel.ToEP.Receiver] {
				recvIsType++
			}
			recvHist[rel.ToEP.Receiver]++
		}
	}

	fmt.Printf("total calls: %d\n", total)
	fmt.Printf("with receiver: %d (%.1f%%)\n", withRecv, 100*float64(withRecv)/float64(total))
	fmt.Printf("receiver is type: %d (%.1f%% of total, %.1f%% of with-recv)\n",
		recvIsType,
		100*float64(recvIsType)/float64(total),
		100*float64(recvIsType)/float64(withRecv))

	// Top 30 receivers.
	type kv struct {
		k string
		v int
	}
	rows := make([]kv, 0, len(recvHist))
	for k, v := range recvHist {
		rows = append(rows, kv{k, v})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].v > rows[j].v })
	fmt.Println("\nTop 30 receivers:")
	for i, r := range rows {
		if i >= 30 {
			break
		}
		mark := " "
		if typeSet[r.k] {
			mark = "T"
		}
		fmt.Printf("  %s %5d  %s\n", mark, r.v, r.k)
	}
}
