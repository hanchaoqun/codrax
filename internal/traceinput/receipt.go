package traceinput

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type preparationReceipt struct {
	Version          string             `json:"version"`
	SourcePath       string             `json:"source_path"`
	SourceKind       string             `json:"source_kind"`
	SourceBytes      int64              `json:"source_bytes"`
	SourceSHA256     string             `json:"source_sha256"`
	SourceGeneration string             `json:"source_generation"`
	QueryPath        string             `json:"query_path"`
	PreviewPath      string             `json:"preview_path"`
	Conversion       hitraceconv.Result `json:"conversion"`
}

func writeReceipt(ctx context.Context, path string, receipt preparationReceipt, bindings map[string]filegeneration.Identity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encodeErr := json.NewEncoder(file).Encode(receipt)
	id, identityErr := filegeneration.FromFile(file)
	if identityErr == nil {
		identityErr = validateHeld(path, file, id)
	}
	err = errors.Join(encodeErr, identityErr, file.Close())
	if err != nil {
		return err
	}
	bindings[path] = id
	return ctx.Err()
}

func bindArtifact(ctx context.Context, artifact hitraceconv.Artifact, bindings map[string]filegeneration.Identity) (err error) {
	path := artifact.Path
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("converter artifact has no clean absolute path: %q", path)
	}
	file, opened, err := filegeneration.OpenRegularReadOnly(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	count, sha, measured, err := tracebundle.MeasureFile(ctx, file)
	if err != nil {
		return err
	}
	if !opened.SameVersion(measured) || count != artifact.Bytes || artifact.SHA256 != "" && artifact.SHA256 != sha {
		return fmt.Errorf("converter artifact no longer matches its receipt: %q", path)
	}
	if err := validateHeld(path, file, opened); err != nil {
		return err
	}
	if prior, ok := bindings[path]; ok && !prior.SameVersion(opened) {
		return fmt.Errorf("converter artifact changed: %q", path)
	}
	bindings[path] = opened
	return nil
}

func bindBundle(ctx context.Context, path string, bindings map[string]filegeneration.Identity) (err error) {
	snapshot, err := tracebundle.Open(ctx, path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, snapshot.Close()) }()
	if prior, ok := bindings[path]; ok && !prior.SameVersion(snapshot.Identity()) {
		return fmt.Errorf("trace bundle generation changed: %q", path)
	}
	bindings[path] = snapshot.Identity()
	var manifest struct {
		Systrace  string `json:"systrace"`
		Artifacts []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"artifacts"`
	}
	if err := snapshot.Decode(&manifest); err != nil {
		return err
	}
	paths := []string{manifest.Systrace}
	for _, artifact := range manifest.Artifacts {
		paths = append(paths, artifact.Path)
	}
	for _, child := range paths {
		if child == "" {
			continue
		}
		if !filepath.IsAbs(child) {
			child = filepath.Join(filepath.Dir(path), filepath.FromSlash(child))
		}
		child = filepath.Clean(child)
		id, err := filegeneration.FromPath(child)
		if err != nil {
			return err
		}
		if prior, ok := bindings[child]; ok && !prior.SameVersion(id) {
			return fmt.Errorf("trace bundle child generation changed: %q", child)
		}
		bindings[child] = id
	}
	// This is the existing schema, capture-id, child-digest and text admission
	// authority. A JSON filename or a converter's event count is not authority.
	if err := tracequery.ValidateTraceInputPath(ctx, path); err != nil {
		return err
	}
	return snapshot.Validate()
}
