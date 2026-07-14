package releaseartifact

import (
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"unicode"
)

type PayloadExpectation string

type LinuxRuntimeExpectation string

const (
	PayloadNone    PayloadExpectation = "none"
	PayloadLinux   PayloadExpectation = "linux-amd64"
	PayloadWindows PayloadExpectation = "windows-amd64"

	LinuxRuntimeGlibc  LinuxRuntimeExpectation = "glibc"
	LinuxRuntimeStatic LinuxRuntimeExpectation = "static"
)

type Options struct {
	ArtifactPath  string
	RepoRoot      string
	GOOS          string
	GOARCH        string
	CGOEnabled    string
	Payload       PayloadExpectation
	LinuxRuntime  LinuxRuntimeExpectation
	RequiredTags  []string
	ForbiddenTags []string
}

type streamerManifest struct {
	SourceURL              string             `json:"source_url"`
	UpstreamRef            string             `json:"upstream_ref"`
	Version                string             `json:"version"`
	AcquisitionRepoLicense string             `json:"acquisition_repo_license"`
	LicenseConcluded       string             `json:"license_concluded"`
	RedistributionStatus   string             `json:"redistribution_status"`
	ProductApprovalRef     string             `json:"product_approval_ref"`
	Platforms              []streamerPlatform `json:"platforms"`
}

type streamerPlatform struct {
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
	Path         string `json:"path"`
	SHA256       string `json:"sha256"`
	SizeBytes    int64  `json:"size_bytes"`
	ActualFormat string `json:"actual_format"`
	MinimumGlibc string `json:"minimum_glibc"`
}

type repositoryPayload struct {
	name         PayloadExpectation
	contract     platformContract
	manifest     streamerManifest
	manifestBody []byte
	payloadBody  []byte
}

type platformContract struct {
	name       PayloadExpectation
	goos       string
	goarch     string
	directory  string
	binaryName string
}

var platformContracts = []platformContract{
	{name: PayloadLinux, goos: "linux", goarch: "amd64", directory: "linux-amd64", binaryName: "trace_streamer"},
	{name: PayloadWindows, goos: "windows", goarch: "amd64", directory: "windows-amd64", binaryName: "trace_streamer.exe"},
}

// Verify proves that a final Codrax artifact has the requested build tuple,
// authoritative tags, and exactly one matching trace_streamer payload (or no
// payload). Repository manifests are validated against their payload bytes
// before either byte sequence is accepted as an artifact witness.
func Verify(options Options) error {
	if strings.TrimSpace(options.RepoRoot) == "" {
		return fmt.Errorf("repository root is required")
	}
	payloads, err := loadRepositoryPayloads(options.RepoRoot)
	if err != nil {
		return err
	}
	return verifyArtifactAgainstPayloads(options, payloads)
}

// VerifyCommercialArtifact validates the complete commercial evidence set and
// the final artifact against the same in-memory payload snapshots. This closes
// the preflight-to-artifact gap: a release artifact cannot be accepted merely
// because a changed repository payload is self-consistent after preflight.
func VerifyCommercialArtifact(options Options) error {
	payloads, err := verifiedCommercialRepositoryPayloads(options.RepoRoot)
	if err != nil {
		return err
	}
	return verifyArtifactAgainstPayloads(options, payloads)
}

func loadRepositoryPayloads(repoRoot string) ([]repositoryPayload, error) {
	payloads := make([]repositoryPayload, 0, len(platformContracts))
	for _, contract := range platformContracts {
		payload, err := loadRepositoryPayload(repoRoot, contract)
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, payload)
	}
	return payloads, nil
}

func verifyArtifactAgainstPayloads(options Options, payloads []repositoryPayload) error {
	if strings.TrimSpace(options.ArtifactPath) == "" {
		return fmt.Errorf("artifact path is required")
	}
	if strings.TrimSpace(options.RepoRoot) == "" {
		return fmt.Errorf("repository root is required")
	}
	if options.GOOS == "" || options.GOARCH == "" || options.CGOEnabled == "" {
		return fmt.Errorf("GOOS, GOARCH, and CGOEnabled are required")
	}
	switch options.Payload {
	case PayloadNone, PayloadLinux, PayloadWindows:
	default:
		return fmt.Errorf("unsupported payload expectation %q", options.Payload)
	}
	switch options.LinuxRuntime {
	case "", LinuxRuntimeGlibc, LinuxRuntimeStatic:
	default:
		return fmt.Errorf("unsupported Linux runtime expectation %q", options.LinuxRuntime)
	}
	if options.LinuxRuntime != "" && options.GOOS != "linux" {
		return fmt.Errorf("Linux runtime expectation %q requires GOOS=linux", options.LinuxRuntime)
	}

	artifact, err := readRegularFileSnapshot(options.ArtifactPath, "artifact")
	if err != nil {
		return err
	}
	info, err := buildinfo.Read(bytes.NewReader(artifact))
	if err != nil {
		return fmt.Errorf("read artifact build info: %w", err)
	}
	settings, err := uniqueBuildSettings(info.Settings)
	if err != nil {
		return err
	}
	for key, want := range map[string]string{
		"CGO_ENABLED": options.CGOEnabled,
		"GOARCH":      options.GOARCH,
		"GOOS":        options.GOOS,
	} {
		if got, ok := settings[key]; !ok || got != want {
			return fmt.Errorf("artifact build setting %s=%q present=%t want=%q", key, got, ok, want)
		}
	}
	tags := parseBuildTags(settings["-tags"])
	for _, required := range options.RequiredTags {
		required = strings.TrimSpace(required)
		if required != "" && !tags[required] {
			return fmt.Errorf("artifact build tags %q lack required tag %q", settings["-tags"], required)
		}
	}
	for _, forbidden := range options.ForbiddenTags {
		forbidden = strings.TrimSpace(forbidden)
		if forbidden != "" && tags[forbidden] {
			return fmt.Errorf("artifact build tags %q contain forbidden tag %q", settings["-tags"], forbidden)
		}
	}
	if options.LinuxRuntime != "" {
		if err := verifyLinuxRuntime(artifact, options.GOARCH, options.LinuxRuntime); err != nil {
			return err
		}
	}

	for _, payload := range payloads {
		want := 0
		if options.Payload == payload.name {
			want = 1
		}
		for _, check := range []struct {
			kind string
			body []byte
		}{
			{kind: "payload", body: payload.payloadBody},
			{kind: "manifest", body: payload.manifestBody},
		} {
			if got := bytes.Count(artifact, check.body); got != want {
				return fmt.Errorf("artifact %s %s count=%d want=%d", payload.name, check.kind, got, want)
			}
		}
	}
	return nil
}

func loadRepositoryPayload(repoRoot string, contract platformContract) (repositoryPayload, error) {
	platformDir := filepath.Join(repoRoot, "internal", "hitraceconv", "embedded_trace_streamer", contract.directory)
	entries, err := os.ReadDir(platformDir)
	if err != nil {
		return repositoryPayload{}, fmt.Errorf("read repository payload directory %s: %w", contract.directory, err)
	}
	allowed := map[string]bool{"manifest.json": true, contract.binaryName: true}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !allowed[entry.Name()] {
			return repositoryPayload{}, fmt.Errorf("repository payload directory %s contains unexpected entry %q", contract.directory, entry.Name())
		}
		seen[entry.Name()] = true
	}
	for name := range allowed {
		if !seen[name] {
			return repositoryPayload{}, fmt.Errorf("repository payload directory %s lacks %s", contract.directory, name)
		}
	}

	manifestPath := filepath.Join(platformDir, "manifest.json")
	manifestBody, err := readRegularFileSnapshot(manifestPath, contract.directory+" manifest")
	if err != nil {
		return repositoryPayload{}, err
	}
	manifest, err := decodeStreamerManifest(manifestBody)
	if err != nil {
		return repositoryPayload{}, fmt.Errorf("validate repository manifest %s: %w", contract.directory, err)
	}
	if err := validateManifestMetadata(manifest); err != nil {
		return repositoryPayload{}, fmt.Errorf("validate repository manifest %s: %w", contract.directory, err)
	}
	if len(manifest.Platforms) != 1 {
		return repositoryPayload{}, fmt.Errorf("validate repository manifest %s: platform count=%d want=1", contract.directory, len(manifest.Platforms))
	}
	platform := manifest.Platforms[0]
	if platform.GOOS != contract.goos || platform.GOARCH != contract.goarch {
		return repositoryPayload{}, fmt.Errorf("validate repository manifest %s: platform tuple=%s/%s want=%s/%s", contract.directory, platform.GOOS, platform.GOARCH, contract.goos, contract.goarch)
	}
	if platform.Path != contract.binaryName || filepath.Base(platform.Path) != platform.Path {
		return repositoryPayload{}, fmt.Errorf("validate repository manifest %s: payload path=%q want exact basename %q", contract.directory, platform.Path, contract.binaryName)
	}
	if platform.SizeBytes <= 0 {
		return repositoryPayload{}, fmt.Errorf("validate repository manifest %s: size_bytes must be positive", contract.directory)
	}
	if strings.TrimSpace(platform.ActualFormat) == "" {
		return repositoryPayload{}, fmt.Errorf("validate repository manifest %s: actual_format is required", contract.directory)
	}
	if contract.goos == "linux" {
		if platform.MinimumGlibc != "2.34" {
			return repositoryPayload{}, fmt.Errorf("validate repository manifest %s: minimum_glibc=%q want=2.34", contract.directory, platform.MinimumGlibc)
		}
	} else if platform.MinimumGlibc != "" {
		return repositoryPayload{}, fmt.Errorf("validate repository manifest %s: minimum_glibc must be empty for %s", contract.directory, contract.goos)
	}
	if err := validateCanonicalSHA256(platform.SHA256); err != nil {
		return repositoryPayload{}, fmt.Errorf("validate repository manifest %s: %w", contract.directory, err)
	}

	payloadPath := filepath.Join(platformDir, contract.binaryName)
	payloadBody, err := readRegularFileSnapshot(payloadPath, contract.directory+" payload")
	if err != nil {
		return repositoryPayload{}, err
	}
	if int64(len(payloadBody)) != platform.SizeBytes {
		return repositoryPayload{}, fmt.Errorf("validate repository manifest %s: payload size=%d want=%d", contract.directory, len(payloadBody), platform.SizeBytes)
	}
	sum := sha256.Sum256(payloadBody)
	if got := hex.EncodeToString(sum[:]); got != platform.SHA256 {
		return repositoryPayload{}, fmt.Errorf("validate repository manifest %s: payload sha256=%s want=%s", contract.directory, got, platform.SHA256)
	}
	return repositoryPayload{
		name:         contract.name,
		contract:     contract,
		manifest:     manifest,
		manifestBody: manifestBody,
		payloadBody:  payloadBody,
	}, nil
}

func decodeStreamerManifest(body []byte) (streamerManifest, error) {
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return streamerManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	var manifest streamerManifest
	if err := validateExactJSONFieldNames(body, &manifest); err != nil {
		return streamerManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return streamerManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return streamerManifest{}, fmt.Errorf("manifest contains multiple JSON values")
		}
		return streamerManifest{}, fmt.Errorf("decode manifest trailer: %w", err)
	}
	return manifest, nil
}

func validateManifestMetadata(manifest streamerManifest) error {
	if strings.TrimSpace(manifest.SourceURL) == "" {
		return fmt.Errorf("source_url is required")
	}
	ref := strings.TrimSpace(manifest.UpstreamRef)
	if ref == "" {
		return fmt.Errorf("upstream_ref is required")
	}
	if !strings.Contains(manifest.SourceURL, ref) {
		return fmt.Errorf("source_url must pin upstream_ref")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return fmt.Errorf("version is required")
	}
	if strings.TrimSpace(manifest.AcquisitionRepoLicense) == "" {
		return fmt.Errorf("acquisition_repo_license is required")
	}
	if strings.TrimSpace(manifest.LicenseConcluded) == "" {
		return fmt.Errorf("license_concluded is required")
	}
	status := strings.ToLower(strings.TrimSpace(manifest.RedistributionStatus))
	if status != "blocked" && status != "approved" {
		return fmt.Errorf("redistribution_status must be blocked or approved")
	}
	if strings.TrimSpace(manifest.ProductApprovalRef) == "" {
		return fmt.Errorf("product_approval_ref is required")
	}
	return nil
}

func validateCanonicalSHA256(value string) error {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return fmt.Errorf("sha256 must be %d lowercase hexadecimal characters", sha256.Size*2)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("sha256 is invalid: %w", err)
	}
	return nil
}

// readRegularFileSnapshot rejects symlinks and path-swap races, then returns a
// single byte snapshot. Build-info parsing and payload counting consume this
// same snapshot, so verification never reopens a possibly replaced artifact.
func readRegularFileSnapshot(name, label string) ([]byte, error) {
	body, _, err := readRegularFileSnapshotInfo(name, label)
	return body, err
}

func readRegularFileSnapshotInfo(name, label string) ([]byte, fs.FileInfo, error) {
	before, err := os.Lstat(name)
	if err != nil {
		return nil, nil, fmt.Errorf("stat %s: %w", label, err)
	}
	if !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s is not a regular file: %s mode=%s", label, name, before.Mode())
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", label, err)
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("stat opened %s: %w", label, err)
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, nil, fmt.Errorf("%s changed identity while opening", label)
	}
	body, err := io.ReadAll(file)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", label, err)
	}
	if int64(len(body)) != after.Size() {
		return nil, nil, fmt.Errorf("%s size changed while reading: read=%d stat=%d", label, len(body), after.Size())
	}
	if len(body) == 0 {
		return nil, nil, fmt.Errorf("%s is empty", label)
	}
	return body, after, nil
}

func verifyLinuxRuntime(artifact []byte, goarch string, expectation LinuxRuntimeExpectation) error {
	file, err := elf.NewFile(bytes.NewReader(artifact))
	if err != nil {
		return fmt.Errorf("parse Linux artifact ELF runtime: %w", err)
	}
	defer file.Close()
	wantMachine := map[string]elf.Machine{
		"amd64": elf.EM_X86_64,
		"arm64": elf.EM_AARCH64,
	}[goarch]
	if wantMachine == elf.EM_NONE || file.Machine != wantMachine {
		return fmt.Errorf("Linux artifact ELF machine=%s want GOARCH=%s machine=%s", file.Machine, goarch, wantMachine)
	}
	interpreter, err := elfInterpreter(file)
	if err != nil {
		return err
	}
	libraries, err := file.ImportedLibraries()
	if err != nil {
		return fmt.Errorf("read Linux artifact dynamic libraries: %w", err)
	}
	switch expectation {
	case LinuxRuntimeGlibc:
		wantInterpreter := map[string]string{
			"amd64": "/lib64/ld-linux-x86-64.so.2",
			"arm64": "/lib/ld-linux-aarch64.so.1",
		}[goarch]
		if wantInterpreter == "" {
			return fmt.Errorf("glibc runtime verification does not support GOARCH=%s", goarch)
		}
		if interpreter != wantInterpreter {
			return fmt.Errorf("Linux artifact interpreter=%q want portable glibc interpreter %q", interpreter, wantInterpreter)
		}
		for _, library := range libraries {
			if library == "libc.so.6" {
				return nil
			}
		}
		return fmt.Errorf("Linux glibc artifact dynamic libraries %v lack libc.so.6", libraries)
	case LinuxRuntimeStatic:
		if interpreter != "" || len(libraries) != 0 {
			return fmt.Errorf("Linux static artifact has interpreter=%q dynamic_libraries=%v", interpreter, libraries)
		}
		return nil
	default:
		return fmt.Errorf("unsupported Linux runtime expectation %q", expectation)
	}
}

func minimumRequiredGlibc(artifact []byte) (string, error) {
	file, err := elf.NewFile(bytes.NewReader(artifact))
	if err != nil {
		return "", fmt.Errorf("parse ELF for glibc baseline: %w", err)
	}
	defer file.Close()
	symbols, err := file.ImportedSymbols()
	if err != nil {
		return "", fmt.Errorf("read imported symbols for glibc baseline: %w", err)
	}
	var best string
	var bestParts []int
	for _, symbol := range symbols {
		if !strings.HasPrefix(symbol.Version, "GLIBC_") {
			continue
		}
		version := strings.TrimPrefix(symbol.Version, "GLIBC_")
		parts, err := numericVersionParts(version)
		if err != nil {
			return "", fmt.Errorf("parse required glibc symbol version %q: %w", symbol.Version, err)
		}
		if compareNumericVersions(parts, bestParts) > 0 {
			best = version
			bestParts = parts
		}
	}
	if best == "" {
		return "", fmt.Errorf("ELF declares no GLIBC_* imported symbol versions")
	}
	return best, nil
}

func numericVersionParts(version string) ([]int, error) {
	pieces := strings.Split(version, ".")
	if len(pieces) < 2 {
		return nil, fmt.Errorf("version must contain at least major.minor")
	}
	parts := make([]int, len(pieces))
	for index, piece := range pieces {
		if piece == "" {
			return nil, fmt.Errorf("empty numeric component")
		}
		value, err := strconv.Atoi(piece)
		if err != nil || value < 0 {
			return nil, fmt.Errorf("component %q is not a non-negative integer", piece)
		}
		parts[index] = value
	}
	return parts, nil
}

func compareNumericVersions(left, right []int) int {
	length := len(left)
	if len(right) > length {
		length = len(right)
	}
	for index := 0; index < length; index++ {
		var leftPart, rightPart int
		if index < len(left) {
			leftPart = left[index]
		}
		if index < len(right) {
			rightPart = right[index]
		}
		if leftPart < rightPart {
			return -1
		}
		if leftPart > rightPart {
			return 1
		}
	}
	return 0
}

func elfInterpreter(file *elf.File) (string, error) {
	var interpreter string
	for _, program := range file.Progs {
		if program.Type != elf.PT_INTERP {
			continue
		}
		if interpreter != "" {
			return "", fmt.Errorf("Linux artifact contains multiple PT_INTERP segments")
		}
		body, err := io.ReadAll(program.Open())
		if err != nil {
			return "", fmt.Errorf("read Linux artifact PT_INTERP: %w", err)
		}
		interpreter = strings.TrimRight(string(body), "\x00")
		if interpreter == "" || strings.IndexByte(interpreter, 0) >= 0 {
			return "", fmt.Errorf("Linux artifact PT_INTERP is malformed")
		}
	}
	return interpreter, nil
}

func rejectDuplicateJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	for valueIndex := 0; ; valueIndex++ {
		err := scanJSONValue(decoder, fmt.Sprintf("$[%d]", valueIndex))
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func scanJSONValue(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]string{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key at %s is not a string", path)
			}
			folded := strings.ToLower(key)
			if prior, duplicate := seen[folded]; duplicate {
				return fmt.Errorf("duplicate JSON object key %q at %s conflicts with %q", key, path, prior)
			}
			seen[folded] = key
			if err := scanJSONValue(decoder, path+"."+key); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("JSON object at %s has invalid closing delimiter", path)
		}
		return nil
	case '[':
		index := 0
		for decoder.More() {
			if err := scanJSONValue(decoder, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
			index++
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("JSON array at %s has invalid closing delimiter", path)
		}
		return nil
	default:
		return fmt.Errorf("unexpected JSON delimiter %q at %s", delimiter, path)
	}
}

func validateExactJSONFieldNames(body []byte, target any) error {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	typeOfTarget := reflect.TypeOf(target)
	if typeOfTarget == nil || typeOfTarget.Kind() != reflect.Pointer {
		return fmt.Errorf("exact JSON schema target must be a non-nil pointer")
	}
	return validateExactJSONValue(value, typeOfTarget.Elem(), "$")
}

func validateExactJSONValue(value any, schema reflect.Type, path string) error {
	for schema.Kind() == reflect.Pointer {
		schema = schema.Elem()
	}
	if value == nil {
		return nil
	}
	switch schema.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return nil // the typed decoder reports the scalar/shape mismatch
		}
		fields := make(map[string]reflect.Type, schema.NumField())
		for index := 0; index < schema.NumField(); index++ {
			field := schema.Field(index)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			fields[name] = field.Type
		}
		for key, child := range object {
			fieldType, exact := fields[key]
			if !exact {
				return fmt.Errorf("JSON object key %q at %s is an unknown field or does not exactly match the schema", key, path)
			}
			if err := validateExactJSONValue(child, fieldType, path+"."+key); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		array, ok := value.([]any)
		if !ok {
			return nil
		}
		for index, child := range array {
			if err := validateExactJSONValue(child, schema.Elem(), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func uniqueBuildSettings(settings []debug.BuildSetting) (map[string]string, error) {
	out := make(map[string]string, len(settings))
	for _, setting := range settings {
		if _, duplicate := out[setting.Key]; duplicate {
			return nil, fmt.Errorf("duplicate artifact build setting %q", setting.Key)
		}
		out[setting.Key] = setting.Value
	}
	return out, nil
}

func parseBuildTags(value string) map[string]bool {
	tags := map[string]bool{}
	for _, tag := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	}) {
		tags[tag] = true
	}
	return tags
}
