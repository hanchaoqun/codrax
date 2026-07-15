//go:build linux

package hitraceconv

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"golang.org/x/sys/unix"
)

func tryExternalToolInheritedInputPlatform(
	source conversionInputView,
	profile externalToolInputProfile,
) (*os.File, filegeneration.Identity, bool, error) {
	if profile != externalToolInputVerifiedLinuxFD {
		return nil, filegeneration.Identity{}, false, nil
	}
	fileSource, ok := source.(externalToolWholeFileSource)
	if !ok {
		return nil, filegeneration.Identity{}, false, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			source.DisplayPath(),
			fmt.Errorf("verified Linux FD profile requires an exact whole-file source"),
		)
	}
	var duplicate *os.File
	if err := fileSource.withOpenFile(func(sourceFile *os.File) error {
		fd, err := unix.FcntlInt(sourceFile.Fd(), unix.F_DUPFD_CLOEXEC, 0)
		runtime.KeepAlive(sourceFile)
		if err != nil {
			return fmt.Errorf("duplicate external tool input FD: %w", err)
		}
		duplicate = os.NewFile(uintptr(fd), "codrax-external-tool-input")
		if duplicate == nil {
			return traceDBJoinPreservingSingle(fmt.Errorf("wrap external tool input FD"), unix.Close(fd))
		}
		return nil
	}); err != nil {
		return nil, filegeneration.Identity{}, false, err
	}
	identity, err := filegeneration.FromFile(duplicate)
	if err != nil || !identity.Strong() || !identity.Mode().IsRegular() || identity.Size() != source.Size() {
		if err == nil {
			err = fmt.Errorf("duplicated Linux input FD has no exact regular-file generation")
		}
		return nil, filegeneration.Identity{}, false, traceDBJoinPreservingSingle(err, duplicate.Close())
	}
	usable := linuxExternalToolProcFDUsable(int(duplicate.Fd()))
	if !usable {
		if err := duplicate.Close(); err != nil {
			return nil, filegeneration.Identity{}, false, fmt.Errorf("close unusable Linux input FD lease: %w", err)
		}
		return nil, filegeneration.Identity{}, false, nil
	}
	return duplicate, identity, true, nil
}

func linuxExternalToolProcFDUsable(sourceFD int) bool {
	procFD, err := unix.Open("/proc", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return false
	}
	defer unix.Close(procFD)
	var fs unix.Statfs_t
	if err := unix.Fstatfs(procFD, &fs); err != nil || uint64(fs.Type) != uint64(unix.PROC_SUPER_MAGIC) {
		return false
	}
	link := make([]byte, 64)
	n, err := unix.Readlinkat(procFD, "self", link)
	if err != nil || strings.TrimSpace(string(link[:n])) != strconv.Itoa(os.Getpid()) {
		return false
	}
	selfFD, err := unix.Openat(procFD, "self/fd", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return false
	}
	defer unix.Close(selfFD)
	pidFD, err := unix.Openat(procFD, strconv.Itoa(os.Getpid())+"/fd", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return false
	}
	defer unix.Close(pidFD)
	var selfStat, pidStat unix.Stat_t
	if unix.Fstat(selfFD, &selfStat) != nil || unix.Fstat(pidFD, &pidStat) != nil ||
		selfStat.Dev != pidStat.Dev || selfStat.Ino != pidStat.Ino || selfStat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return false
	}
	var procEntry, source unix.Stat_t
	if unix.Fstatat(selfFD, strconv.Itoa(sourceFD), &procEntry, 0) != nil || unix.Fstat(sourceFD, &source) != nil {
		return false
	}
	return procEntry.Mode&unix.S_IFMT == unix.S_IFREG &&
		procEntry.Dev == source.Dev && procEntry.Ino == source.Ino
}
