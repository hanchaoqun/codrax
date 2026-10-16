package tracecatalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func (c *Catalog) inspectDelivery(ctx context.Context, p *deliveryPublication) ExportDelivery {
	d := ExportDelivery{Receipt: cloneExportReceipt(p.receipt), Status: "package_invalid"}
	if _, err := validateDeliveryPackage(ctx, p); err != nil {
		d.Reason = boundedMessage(err.Error())
		return d
	}
	d.PackageValid = true
	if c.deliveryFingerprint() != p.fingerprint {
		d.Status, d.Reason = "execution_changed", "Current registered plans, attempts, selection or reference bindings differ; this is the original frozen package, not current completion."
		return d
	}
	for _, a := range c.Snapshot().Artifacts {
		if _, err := c.Resolve(ctx, a.ID); err != nil {
			d.Status, d.Reason = "source_invalid", boundedMessage(err.Error())
			return d
		}
	}
	for _, q := range c.Snapshot().Queries {
		for _, a := range q.Attempts {
			b, ok := c.exportBinding(q.ID, a)
			if !ok {
				continue
			} // Original omissions remain omissions, never become complete.
			for _, file := range b.files {
				if _, _, err := readExportFile(ctx, file.root, file.path, &file); err != nil {
					d.Status, d.Reason = "references_changed", boundedMessage(err.Error())
					return d
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		d.Status, d.Reason = "verification_canceled", err.Error()
		return d
	}
	d.CurrentMatch, d.Status = true, "current"
	return d
}

// Reopen through a held host root, refusing package/result symlinks. Every
// published file is verified, not just the manifest or a short filename hash.
func validateDeliveryPackage(ctx context.Context, p *deliveryPublication) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	packagePath := filepath.Dir(p.receipt.ManifestPath)
	if filepath.Base(packagePath) != p.receipt.Target || filepath.Base(filepath.Dir(packagePath)) != "evidence-exports" || p.receipt.ReportPath != filepath.Join(packagePath, "report.md") {
		return nil, fmt.Errorf("frozen package path differs")
	}
	root, err := os.OpenRoot(filepath.Dir(filepath.Dir(packagePath)))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directories := map[string]os.FileInfo{}
	for _, dir := range []string{"evidence-exports", filepath.Join("evidence-exports", p.receipt.Target)} {
		info, err := root.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("frozen package directory is unavailable or symbolic")
		}
		directories[dir] = info
	}
	pkg, err := root.OpenRoot(filepath.Join("evidence-exports", p.receipt.Target))
	if err != nil {
		return nil, err
	}
	defer pkg.Close()
	data, err := readDeliveryFile(ctx, pkg, ExportFile{Path: "manifest.json", SHA256: p.receipt.ManifestSHA256, Bytes: -1}, MaxSnapshotBytes)
	if err != nil {
		return nil, err
	}
	for _, file := range p.files {
		if _, err := readDeliveryFile(ctx, pkg, file, MaxExportFileBytes); err != nil {
			return nil, err
		}
	}
	for path, before := range directories {
		after, err := root.Lstat(path)
		if err != nil || !after.IsDir() || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, after) {
			return nil, fmt.Errorf("frozen package directory changed while verifying")
		}
	}
	return data, nil
}

func readDeliveryFile(ctx context.Context, root *os.Root, file ExportFile, limit int64) ([]byte, error) {
	if !filepath.IsLocal(file.Path) || filepath.Clean(file.Path) != file.Path {
		return nil, fmt.Errorf("invalid package member path")
	}
	parts := strings.Split(filepath.ToSlash(file.Path), "/")
	for i := range parts {
		name := filepath.Join(parts[:i+1]...)
		info, err := root.Lstat(name)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || i < len(parts)-1 && !info.IsDir() || i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("package member is symbolic or not a directory")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := root.Open(file.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	before, err := filegeneration.FromFile(f)
	if err != nil || !before.Mode().IsRegular() || before.Size() > limit {
		return nil, fmt.Errorf("package member is not a bounded regular file")
	}
	if file.Bytes >= 0 && before.Size() != file.Bytes {
		return nil, fmt.Errorf("package member size changed: %s", file.Path)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > int(limit) {
		return nil, fmt.Errorf("package member exceeds limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	after, err := filegeneration.FromFile(f)
	if err != nil || !before.SameVersion(after) {
		return nil, fmt.Errorf("package member changed while reading")
	}
	info, err := root.Lstat(file.Path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !before.MatchesInfo(info) {
		return nil, fmt.Errorf("package member path changed while reading")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != file.SHA256 {
		return nil, fmt.Errorf("package member SHA256 changed: %s", file.Path)
	}
	return data, nil
}
