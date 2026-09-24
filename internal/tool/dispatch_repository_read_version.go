package tool

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tool/width"
	"github.com/hanchaoqun/codrax/internal/types"
)

// repositoryFileReadVersion is a pending, write-only inspection receipt. Its
// digest comes from the very bytes rendered by read_file, never a later reread.
// It is neither permission to register a test nor evidence that a test passed.
type repositoryFileReadVersion struct {
	mutable                    *types.MutableState
	generation                 uint64
	sourceRoot, sourcePath     string
	physicalRoot, physicalPath string
	rootInfo, fileInfo         os.FileInfo
	digest                     string
	coveragePath               string
	totalLines                 int
}

func beginRepositoryFileReadVersion(ctx *types.BusContext, sourceRoot, requestedPath, fsPath string) repositoryFileReadVersion {
	if ctx == nil || ctx.Mutable == nil || !ctx.Mode.IsWrite() || readFileTypedSourcePath(ctx, requestedPath, fsPath) == "" {
		return repositoryFileReadVersion{}
	}
	read := repositoryFileReadVersion{
		mutable: ctx.Mutable, generation: ctx.Mutable.BeginDispatchRepositoryFileRead(),
		sourceRoot: sourceRoot, sourcePath: fsPath,
		physicalRoot: repositoryReadPhysicalIdentity(sourceRoot), physicalPath: repositoryReadPhysicalIdentity(fsPath),
		coveragePath: readFileTypedSourcePath(ctx, requestedPath, fsPath),
	}
	if read.physicalRoot == "" || read.physicalPath == "" {
		return repositoryFileReadVersion{}
	}
	rel, within := repoRelativePathWithinRoot(read.physicalRoot, read.physicalPath)
	if !within || rel == "" || rel == ".codrax" || strings.HasPrefix(rel, ".codrax/") || types.RuntimeArtifactPathKind(rel) != "" {
		return repositoryFileReadVersion{}
	}
	var err error
	read.rootInfo, err = os.Stat(read.physicalRoot)
	if err != nil || !read.rootInfo.IsDir() {
		return repositoryFileReadVersion{}
	}
	read.fileInfo, err = os.Stat(read.physicalPath)
	if err != nil || !read.fileInfo.Mode().IsRegular() {
		return repositoryFileReadVersion{}
	}
	return read
}

// The descriptor binds the bytes to the observed file rather than to a second
// path-based open. Non-write and ineligible reads retain the existing reader.
func readRepositoryFileVersion(ctx *types.BusContext, sourceRoot, requestedPath, fsPath string, maxBytes int64) ([]byte, repositoryFileReadVersion, error) {
	read := beginRepositoryFileReadVersion(ctx, sourceRoot, requestedPath, fsPath)
	if read.mutable == nil {
		data, err := width.ReadFileBounded(fsPath, maxBytes)
		return data, read, err
	}
	if maxBytes <= 0 {
		maxBytes = width.SourceReadMaxBytes
	}
	f, err := os.Open(fsPath)
	if err != nil {
		return nil, repositoryFileReadVersion{}, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, repositoryFileReadVersion{}, err
	}
	if before.Size() > maxBytes {
		return nil, repositoryFileReadVersion{}, &width.ErrSourceReadOversized{Path: fsPath, Size: before.Size(), Cap: maxBytes}
	}
	limit := maxBytes
	if limit < math.MaxInt64 {
		limit++
	}
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return nil, repositoryFileReadVersion{}, err
	}
	if int64(len(data)) > maxBytes {
		return nil, repositoryFileReadVersion{}, &width.ErrSourceReadOversized{Path: fsPath, Size: int64(len(data)), Cap: maxBytes}
	}
	after, err := f.Stat()
	if err != nil || !sameRepositoryReadFile(read.fileInfo, before) || !sameRepositoryReadFile(before, after) || int64(len(data)) != before.Size() {
		// Preserve ordinary read behavior; unstable identity withholds only the
		// new receipt. It must never trigger model retries or change a verdict.
		return data, repositoryFileReadVersion{}, nil
	}
	if !utf8.Valid(data) {
		// JSON replaces invalid UTF-8 in string output. The read still succeeds,
		// but the rendered text cannot attest inspection of those original bytes.
		return data, repositoryFileReadVersion{}, nil
	}
	sum := sha256.Sum256(data)
	read.digest = hex.EncodeToString(sum[:])
	// Same physical-line definition as textfmt.PhysicalLines, without copying
	// the file again: LF terminates a line; a nonempty suffix is one more.
	read.totalLines = bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		read.totalLines++
	}
	return data, read, nil
}

func sameRepositoryReadFile(a, b os.FileInfo) bool {
	return a != nil && b != nil && a.Mode().IsRegular() && b.Mode().IsRegular() &&
		os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

func (read repositoryFileReadVersion) record(ctx *types.BusContext, result types.ToolResult, displayComplete bool) {
	if read.mutable == nil || ctx == nil || ctx.Mutable != read.mutable || !ctx.Mode.IsWrite() || !displayComplete ||
		!result.Success || result.ToolName != "read_file" || result.RuntimeArtifactRead != nil || result.ReadCoverage == nil || read.digest == "" {
		return
	}
	coverage := result.ReadCoverage
	path, ok := protectedBaselineExactPath(coverage.Path)
	if !ok || coverage.Path != read.coveragePath || coverage.TotalLines != read.totalLines ||
		result.RawRef == "" || coverage.RawRef != result.RawRef ||
		repositoryReadPhysicalIdentity(read.sourceRoot) != read.physicalRoot || repositoryReadPhysicalIdentity(read.sourcePath) != read.physicalPath {
		return
	}
	rootInfo, err := os.Stat(read.physicalRoot)
	if err != nil || !rootInfo.IsDir() || !os.SameFile(read.rootInfo, rootInfo) {
		return
	}
	fileInfo, err := os.Stat(read.physicalPath)
	if err != nil || !sameRepositoryReadFile(read.fileInfo, fileInfo) {
		return
	}
	read.mutable.RecordDispatchRepositoryFileReadVersionWithSummary(read.generation, read.physicalRoot, path, result.RawRef,
		read.digest, coverage.LineStart, coverage.LineEnd, coverage.TotalLines, result.Summary)
}
