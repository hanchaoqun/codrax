// Package immutablefile publishes completed bytes without replacing an existing
// content-addressed leaf. It supplies no execution, query or evidence authority.
package immutablefile

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func validLeaf(leaf string) bool {
	return leaf != "" && leaf != "." && leaf != ".." && !filepath.IsAbs(leaf) && !strings.ContainsAny(leaf, "/\\\x00")
}

// Publish reuses only identical regular material with a stable held/path
// generation. The caller owns naming and the trusted root; neither is inferred.
func Publish(dir, leaf string, data []byte) (string, error) {
	if !PublicationSupported {
		return "", fmt.Errorf("atomic no-replace result publication unsupported on this platform")
	}
	if strings.TrimSpace(dir) == "" || !validLeaf(leaf) {
		return "", fmt.Errorf("invalid immutable material root or leaf")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	path := filepath.Join(dir, leaf)
	if _, err := root.Lstat(leaf); err == nil {
		return path, Matches(root, leaf, data)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	stage, err := stageBytes(root, data)
	if err != nil {
		return "", err
	}
	defer root.Remove(stage)
	parent, err := root.Open(".")
	if err != nil {
		return "", err
	}
	defer parent.Close()
	if err = RenameNoReplace(parent, dir, stage, leaf); err != nil && !os.IsExist(err) {
		return "", err
	}
	// No link/unlink: unlink changes ctime after another reader may have bound it.
	if err := Matches(root, leaf, data); err != nil {
		return "", err
	}
	return path, nil
}

func stageBytes(root *os.Root, data []byte) (string, error) {
	stage := ".immutable-material-" + rand.Text()
	f, err := root.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return "", err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = root.Remove(stage)
		return "", err
	}
	return stage, nil
}

func openRegular(root *os.Root, leaf string) (*os.File, filegeneration.Identity, error) {
	if !validLeaf(leaf) {
		return nil, filegeneration.Identity{}, fmt.Errorf("invalid material leaf")
	}
	info, err := root.Lstat(leaf)
	if err != nil {
		return nil, filegeneration.Identity{}, err
	}
	if !info.Mode().IsRegular() {
		return nil, filegeneration.Identity{}, fmt.Errorf("material is not regular")
	}
	f, err := root.OpenFile(leaf, os.O_RDONLY|ReadFlags, 0)
	if err != nil {
		return nil, filegeneration.Identity{}, err
	}
	before, err := filegeneration.FromFile(f)
	opened, statErr := f.Stat()
	if err != nil || statErr != nil || !os.SameFile(info, opened) || !before.Mode().IsRegular() {
		f.Close()
		return nil, filegeneration.Identity{}, fmt.Errorf("material changed before verification")
	}
	return f, before, nil
}

func stable(root *os.Root, leaf string, f *os.File, before filegeneration.Identity) error {
	after, err := filegeneration.FromFile(f)
	if err != nil || !before.SameVersion(after) {
		return fmt.Errorf("material generation changed during verification")
	}
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	current, err := root.Lstat(leaf)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return fmt.Errorf("material path replaced during verification")
	}
	currentVersion, err := filegeneration.FromPath(filepath.Join(root.Name(), leaf))
	if err != nil || !before.SameVersion(currentVersion) {
		return fmt.Errorf("material path generation changed during verification")
	}
	return nil
}

// Matches checks all bytes without allocating another full payload.
func Matches(root *os.Root, leaf string, data []byte) error {
	f, before, err := openRegular(root, leaf)
	if err != nil {
		return err
	}
	defer f.Close()
	if before.Size() != int64(len(data)) {
		return fmt.Errorf("existing material size conflicts with content address")
	}
	buf := make([]byte, 32*1024)
	for offset := 0; offset < len(data); {
		n := min(len(buf), len(data)-offset)
		if _, err := io.ReadFull(f, buf[:n]); err != nil {
			return err
		}
		if string(buf[:n]) != string(data[offset:offset+n]) {
			return fmt.Errorf("existing material bytes conflict with content address")
		}
		offset += n
	}
	if n, err := f.Read(buf[:1]); n != 0 || err != io.EOF {
		return fmt.Errorf("material grew during verification")
	}
	return stable(root, leaf, f, before)
}

// Read requires an exact regular leaf inside the supplied root; no basename
// cleanup, symlink following, generation relaxation or fallback is performed.
func Read(dir, leaf string) ([]byte, error) {
	if strings.TrimSpace(dir) == "" || !validLeaf(leaf) {
		return nil, fmt.Errorf("invalid material root or leaf")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, before, err := openRegular(root, leaf)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, before.Size()+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != before.Size() {
		return nil, fmt.Errorf("material size changed during read")
	}
	if err := stable(root, leaf, f, before); err != nil {
		return nil, err
	}
	return data, nil
}

// ReplaceCurrent is explicitly mutable compatibility publication, not immutable
// publication. It writes through a unique completed temporary file and keeps an
// identical existing generation unchanged. Callers must never pass archive IDs.
func ReplaceCurrent(dir, leaf string, data []byte) error {
	if strings.TrimSpace(dir) == "" || !validLeaf(leaf) {
		return fmt.Errorf("invalid current material root or leaf")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if info, err := root.Lstat(leaf); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("current material is not regular")
		}
		if err := Matches(root, leaf, data); err == nil {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	stage, err := stageBytes(root, data)
	if err != nil {
		return err
	}
	defer root.Remove(stage)
	if info, err := root.Lstat(leaf); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("current material is not regular")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := root.Rename(stage, leaf); err != nil {
		return err
	}
	return Matches(root, leaf, data)
}
