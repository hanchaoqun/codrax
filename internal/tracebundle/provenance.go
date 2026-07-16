package tracebundle

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

const (
	// SchemaV2 is the exact wire-schema identity for manifests which bind every
	// causal child to byte-exact content provenance.
	SchemaV2 = "codrax.tracebundle/v2"

	provenanceReadBufferBytes = 256 << 10
	captureIDDomain           = "codrax.tracebundle.capture/v2\x00"
)

// CaptureMember is the complete hard provenance tuple for one causal child.
// Path is the canonical, slash-separated bundle-relative wire spelling, not a
// host filesystem path.
type CaptureMember struct {
	Type   string
	Path   string
	Bytes  int64
	SHA256 string
}

// ValidateSHA256 accepts only the canonical wire spelling used by schema v2:
// exactly 64 lowercase hexadecimal characters without an algorithm prefix.
func ValidateSHA256(value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("sha256 must be exactly %d lowercase hexadecimal characters", sha256.Size*2)
	}
	if value != strings.ToLower(value) {
		return fmt.Errorf("sha256 must use lowercase hexadecimal")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("sha256 is not canonical hexadecimal")
	}
	return nil
}

// ValidateCaptureID checks the exact domain-tagged identity spelling. The
// digest itself is recomputed by CaptureID; this helper validates syntax only.
func ValidateCaptureID(value string) error {
	const prefix = "sha256:"
	if !strings.HasPrefix(value, prefix) {
		return fmt.Errorf("capture_id must use the sha256: prefix")
	}
	if err := ValidateSHA256(strings.TrimPrefix(value, prefix)); err != nil {
		return fmt.Errorf("capture_id: %w", err)
	}
	return nil
}

// ValidateCapturePath checks the portable wire-path contract. Schema-v2
// causal members are resolved only beneath the bundle directory, so absolute,
// non-canonical, backslash and parent-escape spellings are rejected here.
func ValidateCapturePath(value string) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("capture member path must be non-empty and unpadded")
	}
	if strings.Contains(value, "\\") {
		return fmt.Errorf("capture member path must use slash separators")
	}
	if strings.HasPrefix(value, "/") || path.IsAbs(value) {
		return fmt.Errorf("capture member path must be bundle-relative")
	}
	clean := path.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("capture member path is not canonical bundle-relative syntax")
	}
	// A drive/UNC-like prefix is not portable even on a host where path.IsAbs
	// treats it as an ordinary relative string.
	if len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':' {
		return fmt.Errorf("capture member path must not contain a volume prefix")
	}
	return nil
}

// CaptureID returns the deterministic identity of the complete causal child
// set. It is a reconciliation identity, not a signature or authenticity
// claim. Input order is deliberately irrelevant.
func CaptureID(members []CaptureMember) (string, error) {
	canonical := append([]CaptureMember(nil), members...)
	paths := make(map[string]struct{}, len(canonical))
	for i := range canonical {
		if canonical[i].Type != "systrace" && canonical[i].Type != "perftrace" {
			return "", fmt.Errorf("capture member %d has unsupported causal type %q", i, canonical[i].Type)
		}
		if err := ValidateCapturePath(canonical[i].Path); err != nil {
			return "", fmt.Errorf("capture member %d path: %w", i, err)
		}
		if canonical[i].Bytes < 0 {
			return "", fmt.Errorf("capture member %d has negative byte size", i)
		}
		if err := ValidateSHA256(canonical[i].SHA256); err != nil {
			return "", fmt.Errorf("capture member %d: %w", i, err)
		}
		if _, duplicate := paths[canonical[i].Path]; duplicate {
			return "", fmt.Errorf("duplicate causal child path %q", canonical[i].Path)
		}
		paths[canonical[i].Path] = struct{}{}
	}
	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].Type != canonical[j].Type {
			return canonical[i].Type < canonical[j].Type
		}
		if canonical[i].Path != canonical[j].Path {
			return canonical[i].Path < canonical[j].Path
		}
		if canonical[i].Bytes != canonical[j].Bytes {
			return canonical[i].Bytes < canonical[j].Bytes
		}
		return canonical[i].SHA256 < canonical[j].SHA256
	})
	digest := sha256.New()
	_, _ = io.WriteString(digest, captureIDDomain)
	for _, member := range canonical {
		writeCaptureString(digest, member.Type)
		writeCaptureString(digest, member.Path)
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(member.Bytes))
		_, _ = digest.Write(size[:])
		decoded, _ := hex.DecodeString(member.SHA256) // validated above
		_, _ = digest.Write(decoded)
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func writeCaptureString(writer io.Writer, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = writer.Write(size[:])
	_, _ = io.WriteString(writer, value)
}

// ValidateFile proves that file still denotes expectedIdentity. Callers own
// the descriptor and decide whether path-binding validation is also required.
func ValidateFile(ctx context.Context, file *os.File, expectedIdentity filegeneration.Identity) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if file == nil {
		return fmt.Errorf("provenance file is nil")
	}
	if !expectedIdentity.Initialized() || !expectedIdentity.Strong() || !expectedIdentity.Mode().IsRegular() || expectedIdentity.Size() < 0 {
		return fmt.Errorf("expected provenance identity is not a strong regular-file generation")
	}
	current, err := filegeneration.FromFile(file)
	if err != nil {
		return fmt.Errorf("measure provenance file identity: %w", err)
	}
	if !current.Strong() || !current.Mode().IsRegular() || !expectedIdentity.SameVersion(current) {
		return fmt.Errorf("provenance file generation changed")
	}
	return nil
}

// MeasureFile streams exactly one held regular-file generation through
// SHA-256 with bounded memory and cooperative cancellation. It validates the
// complete strong identity before and after the read and never reopens Name().
func MeasureFile(ctx context.Context, file *os.File) (bytesRead int64, sha string, identity filegeneration.Identity, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, "", filegeneration.Identity{}, err
	}
	if file == nil {
		return 0, "", filegeneration.Identity{}, fmt.Errorf("provenance file is nil")
	}
	identity, err = filegeneration.FromFile(file)
	if err != nil {
		return 0, "", filegeneration.Identity{}, fmt.Errorf("capture provenance file identity: %w", err)
	}
	if !identity.Strong() || !identity.Mode().IsRegular() || identity.Size() < 0 {
		return 0, "", filegeneration.Identity{}, fmt.Errorf("provenance source is not a strong regular-file generation")
	}

	hash := sha256.New()
	buffer := make([]byte, provenanceReadBufferBytes)
	for offset := int64(0); offset < identity.Size(); {
		if err := ctx.Err(); err != nil {
			return 0, "", filegeneration.Identity{}, err
		}
		want := int64(len(buffer))
		if remaining := identity.Size() - offset; remaining < want {
			want = remaining
		}
		n, readErr := file.ReadAt(buffer[:int(want)], offset)
		if n != int(want) {
			if readErr == nil {
				readErr = io.ErrUnexpectedEOF
			}
			return 0, "", filegeneration.Identity{}, fmt.Errorf("read provenance file at offset %d: got=%d want=%d: %w", offset, n, want, readErr)
		}
		if readErr != nil && readErr != io.EOF {
			return 0, "", filegeneration.Identity{}, fmt.Errorf("read provenance file at offset %d: %w", offset, readErr)
		}
		_, _ = hash.Write(buffer[:n])
		offset += int64(n)
	}
	if err := ctx.Err(); err != nil {
		return 0, "", filegeneration.Identity{}, err
	}
	if err := ValidateFile(ctx, file, identity); err != nil {
		return 0, "", filegeneration.Identity{}, err
	}
	return identity.Size(), hex.EncodeToString(hash.Sum(nil)), identity, nil
}
