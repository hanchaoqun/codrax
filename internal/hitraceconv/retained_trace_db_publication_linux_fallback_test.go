//go:build linux

package hitraceconv

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLinuxUnnamedPublicationFallbackRequiresTypedCapabilityFailure(t *testing.T) {
	for _, err := range []error{unix.EOPNOTSUPP, unix.ENOTSUP, unix.EINVAL, unix.ENOSYS, unix.EISDIR} {
		if !linuxUnnamedPublicationFallbackAllowed(err) {
			t.Fatalf("typed unnamed-inode capability failure did not enable compatibility publication: %v", err)
		}
	}
	for _, err := range []error{unix.EACCES, unix.EPERM, unix.EIO, unix.ENOSPC, errors.New("untyped failure")} {
		if linuxUnnamedPublicationFallbackAllowed(err) {
			t.Fatalf("non-capability failure incorrectly enabled compatibility publication: %v", err)
		}
	}
}

func TestLinuxSealedPublicationNamedFallbackPublishesNoReplaceAndCleansTemp(t *testing.T) {
	parent := t.TempDir()
	finalPath := filepath.Join(parent, "capture.systrace")
	parentFD, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(parentFD)
	body := bytes.Repeat([]byte("sealed DrvFS-compatible output\n"), 128)
	tempFD, tempLeaf, err := openLinuxNamedPublicationTemp(parentFD, sealedConversionPublicationOutput)
	if err != nil {
		t.Fatal(err)
	}
	temp := os.NewFile(uintptr(tempFD), tempLeaf)
	if temp == nil {
		unix.Close(tempFD)
		t.Fatal("wrap named compatibility generation")
	}
	defer temp.Close()
	defer unix.Unlinkat(parentFD, tempLeaf, 0)
	if _, err := temp.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := temp.Chmod(0o640); err != nil {
		t.Fatal(err)
	}
	if err := temp.Sync(); err != nil {
		t.Fatal(err)
	}
	err = publishLinuxNamedPublicationTemp(parentFD, tempLeaf, filepath.Base(finalPath), temp, sealedConversionPublicationOutput)
	if err != nil {
		t.Fatalf("publish through named compatibility path: %v", err)
	}
	got, err := os.ReadFile(finalPath)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("published body mismatch: bytes=%d err=%v", len(got), err)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(finalPath) {
		t.Fatalf("named compatibility staging survived publication: %+v", entries)
	}
}

func TestLinuxSealedPublicationNamedFallbackPreservesRacingFinalOwner(t *testing.T) {
	parent := t.TempDir()
	finalPath := filepath.Join(parent, "capture.systrace")
	parentFD, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(parentFD)
	tempFD, tempLeaf, err := openLinuxNamedPublicationTemp(parentFD, sealedConversionPublicationOutput)
	if err != nil {
		t.Fatal(err)
	}
	temp := os.NewFile(uintptr(tempFD), tempLeaf)
	if temp == nil {
		unix.Close(tempFD)
		t.Fatal("wrap named compatibility generation")
	}
	defer temp.Close()
	defer unix.Unlinkat(parentFD, tempLeaf, 0)
	if _, err := temp.Write([]byte("private generation")); err != nil {
		t.Fatal(err)
	}
	external := []byte("racing external owner")
	if err := os.WriteFile(finalPath, external, 0o600); err != nil {
		t.Fatal(err)
	}
	err = publishLinuxNamedPublicationTemp(parentFD, tempLeaf, filepath.Base(finalPath), temp, sealedConversionPublicationOutput)
	if err == nil || !strings.Contains(err.Error(), "atomically") {
		t.Fatalf("named compatibility collision did not fail closed: %v", err)
	}
	// The production caller owns this cleanup in its outer resultErr defer.
	if cleanupErr := removeLinuxNamedPublicationTemp(parentFD, tempLeaf, temp, sealedConversionPublicationOutput); cleanupErr != nil {
		t.Fatalf("clean failed named compatibility generation: %v", cleanupErr)
	}
	got, readErr := os.ReadFile(finalPath)
	if readErr != nil || !bytes.Equal(got, external) {
		t.Fatalf("collision changed the external owner: got=%q err=%v", got, readErr)
	}
	entries, readDirErr := os.ReadDir(parent)
	if readDirErr != nil {
		t.Fatal(readDirErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".codrax-publish-") {
			t.Fatalf("failed named compatibility publication leaked staging: %q", entry.Name())
		}
	}
}
