package tool

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// Native query results are completed bytes, not a query cache or an authority
// carrier. Repeated producers may reuse identical material without changing a
// prior attempt's file generation. Other tools and attached-input repair retain
// their existing policies. Never repair/overwrite a conflicting result file.
func storeImmutableTraceResult(workDir, nameHint, output string) string {
	if strings.TrimSpace(workDir) == "" {
		return ""
	}
	path, err := publishImmutableTraceResult(workDir, nameHint, output)
	if err != nil {
		logging.Warning("[trace_query] immutable result publication failed: %v", err)
		return ""
	}
	return path
}

func immutableTraceResultName(nameHint, output string) string {
	ext := filepath.Ext(nameHint)
	if ext == "" {
		ext = ".txt"
	}
	// Both components are leaves, never caller-controlled path segments.
	ext = "." + sanitizeToolName(strings.TrimPrefix(ext, "."))
	stem := sanitizeToolName(strings.TrimSuffix(firstNonEmptyBlobName(nameHint, "trace_query"), filepath.Ext(nameHint)))
	if len(stem) > 120 {
		stem = stem[:120]
	}
	if len(ext) > 20 {
		ext = ".txt"
	}
	sum := sha256.Sum256([]byte(output))
	return stem + "-" + hex.EncodeToString(sum[:]) + ext
}

func publishImmutableTraceResult(workDir, nameHint, output string) (string, error) {
	if !immutableBlobPublicationSupported {
		return "", fmt.Errorf("atomic no-replace result publication unsupported on this platform")
	}
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(workDir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	leaf := immutableTraceResultName(nameHint, output)
	path := filepath.Join(workDir, leaf)
	if _, err := root.Lstat(leaf); err == nil {
		return path, immutableTraceResultMatches(root, leaf, output)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	stage := ".trace-result-" + rand.Text()
	f, err := root.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return "", err
	}
	defer root.Remove(stage)
	if _, err = io.WriteString(f, output); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	parent, err := root.Open(".")
	if err != nil {
		return "", err
	}
	defer parent.Close()
	if err = immutableBlobRenameNoReplace(parent, workDir, stage, leaf); err != nil && !os.IsExist(err) {
		return "", err
	}
	// A racing winner is reusable only after complete bytes and the held/path
	// generations agree. No Link+unlink: unlink would change ctime after another
	// publisher had already captured the winner's generation.
	if err := immutableTraceResultMatches(root, leaf, output); err != nil {
		return "", err
	}
	return path, nil
}

func immutableTraceResultMatches(root *os.Root, leaf, output string) error {
	info, err := root.Lstat(leaf)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != int64(len(output)) {
		return fmt.Errorf("existing result is not identical regular material")
	}
	f, err := root.OpenFile(leaf, os.O_RDONLY|immutableBlobReadFlags, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	before, err := filegeneration.FromFile(f)
	if err != nil {
		return err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !before.Mode().IsRegular() || before.Size() != int64(len(output)) {
		return fmt.Errorf("result changed before verification")
	}
	buf := make([]byte, 32*1024)
	for offset := 0; offset < len(output); {
		n := min(len(buf), len(output)-offset)
		if _, err := io.ReadFull(f, buf[:n]); err != nil {
			return err
		}
		if string(buf[:n]) != output[offset:offset+n] {
			return fmt.Errorf("existing result bytes conflict with content address")
		}
		offset += n
	}
	if n, err := f.Read(buf[:1]); n != 0 || err != io.EOF {
		return fmt.Errorf("result grew during verification")
	}
	after, err := filegeneration.FromFile(f)
	if err != nil || !before.SameVersion(after) {
		return fmt.Errorf("result generation changed during verification")
	}
	current, err := root.Lstat(leaf)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return fmt.Errorf("result path replaced during verification")
	}
	// Reopen by the returned path as well: a detached/replaced producer root
	// must not yield a reference to another file.
	currentVersion, err := filegeneration.FromPath(filepath.Join(root.Name(), leaf))
	if err != nil || !before.SameVersion(currentVersion) {
		return fmt.Errorf("result path generation changed during verification")
	}
	return nil
}
