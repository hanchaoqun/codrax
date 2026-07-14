//go:build !windows

package filegeneration

import "os"

func enhanceIdentityFromFile(_ *os.File, _ os.FileInfo, id Identity) Identity {
	return id
}
