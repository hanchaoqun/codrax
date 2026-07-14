package releaseartifact

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRepositoryCommercialTraceStreamerReleaseIsFailLoudWhileBlocked(t *testing.T) {
	repo := releaseArtifactRepositoryRoot(t)
	err := VerifyCommercialTraceStreamerRelease(repo)
	if err == nil || !strings.Contains(err.Error(), "commercial redistribution is blocked") || !strings.Contains(err.Error(), "NOASSERTION") {
		t.Fatalf("repository commercial gate error=%v, want explicit NOASSERTION/blocked rejection", err)
	}
}

func TestCommercialTraceStreamerReleaseRequiresCompletePayloadScopedEvidence(t *testing.T) {
	repo, provenance := commercialTestRepository(t, true)
	if err := VerifyCommercialTraceStreamerRelease(repo); err != nil {
		t.Fatalf("complete approved fixture rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*commercialProvenance)
		want   string
	}{
		{name: "legal approval", mutate: func(p *commercialProvenance) { p.CommercialReleaseEvidence.PayloadScopedLegalApproval = nil }, want: "lacks payload_scoped_legal_approval"},
		{name: "sbom", mutate: func(p *commercialProvenance) { p.CommercialReleaseEvidence.SBOM = nil }, want: "lacks sbom"},
		{name: "license bundle", mutate: func(p *commercialProvenance) { p.CommercialReleaseEvidence.DependencyLicenseBundle = nil }, want: "lacks dependency_license_bundle"},
		{name: "notices", mutate: func(p *commercialProvenance) { p.CommercialReleaseEvidence.ThirdPartyNotices = nil }, want: "lacks third_party_notices"},
		{name: "build attestation", mutate: func(p *commercialProvenance) { p.CommercialReleaseEvidence.SourceBuildAttestation = nil }, want: "lacks source_build_attestation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copy := provenance
			test.mutate(&copy)
			writeCommercialProvenance(t, repo, copy)
			err := VerifyCommercialTraceStreamerRelease(repo)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("commercial gate error=%v, want containing %q", err, test.want)
			}
			writeCommercialProvenance(t, repo, provenance)
		})
	}
}

func TestCommercialTraceStreamerReleaseRejectsStaleOrUnboundEvidence(t *testing.T) {
	t.Run("duplicate provenance field", func(t *testing.T) {
		repo, _ := commercialTestRepository(t, true)
		path := filepath.Join(repo, filepath.FromSlash(traceStreamerProvenancePath))
		body := bytes.TrimSpace(mustReadFile(t, path))
		body = append(body[:len(body)-1], []byte(",\n  \"redistribution_status\": \"blocked\"\n}\n")...)
		mustWriteFile(t, path, body, 0o644)
		err := VerifyCommercialTraceStreamerRelease(repo)
		if err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
			t.Fatalf("commercial gate error=%v, want duplicate-key rejection", err)
		}
	})

	t.Run("stale provenance asset", func(t *testing.T) {
		repo, provenance := commercialTestRepository(t, true)
		provenance.Assets[0].SHA256 = strings.Repeat("0", 64)
		writeCommercialProvenance(t, repo, provenance)
		err := VerifyCommercialTraceStreamerRelease(repo)
		if err == nil || !strings.Contains(err.Error(), "does not match repository payload bytes") {
			t.Fatalf("commercial gate error=%v, want stale payload rejection", err)
		}
	})

	t.Run("mutated evidence file", func(t *testing.T) {
		repo, provenance := commercialTestRepository(t, true)
		mustWriteFile(t, filepath.Join(repo, filepath.FromSlash(provenance.CommercialReleaseEvidence.SBOM.Path)), []byte("mutated-sbom"), 0o644)
		err := VerifyCommercialTraceStreamerRelease(repo)
		if err == nil || !strings.Contains(err.Error(), "evidence sbom sha256=") {
			t.Fatalf("commercial gate error=%v, want evidence hash rejection", err)
		}
	})

	t.Run("legal approval payload set", func(t *testing.T) {
		repo, provenance := commercialTestRepository(t, true)
		provenance.CommercialReleaseEvidence.PayloadScopedLegalApproval.PayloadSHA256s = provenance.CommercialReleaseEvidence.PayloadScopedLegalApproval.PayloadSHA256s[:1]
		writeCommercialProvenance(t, repo, provenance)
		err := VerifyCommercialTraceStreamerRelease(repo)
		if err == nil || !strings.Contains(err.Error(), "want exact payload set") {
			t.Fatalf("commercial gate error=%v, want exact payload-set rejection", err)
		}
	})

	t.Run("legal approval evidence set", func(t *testing.T) {
		repo, provenance := commercialTestRepository(t, true)
		body := []byte(`{"spdxVersion":"SPDX-2.3","replacement":true}`)
		path := "third_party/trace_streamer/evidence/replacement.spdx.json"
		mustWriteFile(t, filepath.Join(repo, filepath.FromSlash(path)), body, 0o644)
		provenance.CommercialReleaseEvidence.SBOM = &fileEvidence{Path: path, SHA256: sha256Hex(body)}
		writeCommercialProvenance(t, repo, provenance)
		err := VerifyCommercialTraceStreamerRelease(repo)
		if err == nil || !strings.Contains(err.Error(), "complete evidence path/hash set") {
			t.Fatalf("commercial gate error=%v, want legal evidence-binding rejection", err)
		}
	})

	t.Run("unpinned acquisition ref", func(t *testing.T) {
		repo, provenance := commercialTestRepository(t, true)
		provenance.Acquisition.Ref = "0123456789abcdef0123456789abcdef01234567"
		writeCommercialProvenance(t, repo, provenance)
		err := VerifyCommercialTraceStreamerRelease(repo)
		if err == nil || !strings.Contains(err.Error(), "want pinned") {
			t.Fatalf("commercial gate error=%v, want fixed acquisition-ref rejection", err)
		}
	})

	t.Run("legal document content", func(t *testing.T) {
		repo, provenance := commercialTestRepository(t, true)
		legal := provenance.CommercialReleaseEvidence.PayloadScopedLegalApproval
		document := legalApprovalDocument{
			SchemaVersion:    1,
			Decision:         "approved_for_internal_testing_only",
			LicenseConcluded: provenance.LicenseConcluded,
			ApprovalID:       legal.ApprovalID,
			ApprovedBy:       legal.ApprovedBy,
			ApprovedAt:       legal.ApprovedAt,
			PayloadSHA256s:   legal.PayloadSHA256s,
			Evidence: commercialEvidenceBindings{
				SBOM:                    *provenance.CommercialReleaseEvidence.SBOM,
				DependencyLicenseBundle: *provenance.CommercialReleaseEvidence.DependencyLicenseBundle,
				ThirdPartyNotices:       *provenance.CommercialReleaseEvidence.ThirdPartyNotices,
				SourceBuildAttestation:  *provenance.CommercialReleaseEvidence.SourceBuildAttestation,
			},
		}
		body := marshalJSON(t, document)
		mustWriteFile(t, filepath.Join(repo, filepath.FromSlash(legal.Path)), body, 0o644)
		legal.SHA256 = sha256Hex(body)
		writeCommercialProvenance(t, repo, provenance)
		err := VerifyCommercialTraceStreamerRelease(repo)
		if err == nil || !strings.Contains(err.Error(), "does not bind the approved status") {
			t.Fatalf("commercial gate error=%v, want approval-content rejection", err)
		}
	})

	t.Run("padded NOASSERTION", func(t *testing.T) {
		repo, provenance := commercialTestRepository(t, true)
		provenance.LicenseConcluded = " NOASSERTION "
		for _, contract := range platformContracts {
			mutateManifest(t, repo, contract.name, func(manifest *streamerManifest) {
				manifest.LicenseConcluded = provenance.LicenseConcluded
			})
		}
		writeCommercialProvenance(t, repo, provenance)
		err := VerifyCommercialTraceStreamerRelease(repo)
		if err == nil || !strings.Contains(err.Error(), "canonical without surrounding whitespace") {
			t.Fatalf("commercial gate error=%v, want padded-NOASSERTION rejection", err)
		}
	})
}

func TestCommercialTraceStreamerReleaseBlockedFixtureDoesNotNeedFakeEvidence(t *testing.T) {
	repo, _ := commercialTestRepository(t, false)
	err := VerifyCommercialTraceStreamerRelease(repo)
	if err == nil || !strings.Contains(err.Error(), "commercial redistribution is blocked") {
		t.Fatalf("blocked fixture error=%v", err)
	}
}

func TestCommercialTraceStreamerReleaseRejectsEvidencePathAndIdentityAliases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged Windows test environments do not reliably permit symlink/hardlink creation")
	}

	t.Run("symlinked ancestor", func(t *testing.T) {
		repo, _ := commercialTestRepository(t, true)
		evidenceDir := filepath.Join(repo, filepath.FromSlash(traceStreamerEvidenceDir))
		realEvidenceDir := filepath.Join(repo, "relocated-evidence")
		if err := os.Rename(evidenceDir, realEvidenceDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realEvidenceDir, evidenceDir); err != nil {
			t.Fatal(err)
		}
		err := VerifyCommercialTraceStreamerRelease(repo)
		if err == nil || (!strings.Contains(err.Error(), "symlink or junction") && !strings.Contains(err.Error(), "resolves outside")) {
			t.Fatalf("commercial gate error=%v, want symlinked-ancestor rejection", err)
		}
	})

	t.Run("hardlink role reuse", func(t *testing.T) {
		repo, provenance := commercialTestRepository(t, true)
		sbom := provenance.CommercialReleaseEvidence.SBOM
		notices := provenance.CommercialReleaseEvidence.ThirdPartyNotices
		noticesPath := filepath.Join(repo, filepath.FromSlash(notices.Path))
		if err := os.Remove(noticesPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(filepath.Join(repo, filepath.FromSlash(sbom.Path)), noticesPath); err != nil {
			t.Fatal(err)
		}
		notices.SHA256 = sbom.SHA256
		rewriteLegalApprovalDocument(t, repo, &provenance)
		writeCommercialProvenance(t, repo, provenance)
		err := VerifyCommercialTraceStreamerRelease(repo)
		if err == nil || !strings.Contains(err.Error(), "reuses the physical file") {
			t.Fatalf("commercial gate error=%v, want physical-file reuse rejection", err)
		}
	})
}

func commercialTestRepository(t *testing.T, approved bool) (string, commercialProvenance) {
	t.Helper()
	repo := t.TempDir()
	writeTestRepositoryPayloads(t, repo)
	installCommercialLinuxPayloadFixture(t, repo)
	for _, contract := range platformContracts {
		mutateManifest(t, repo, contract.name, func(manifest *streamerManifest) {
			manifest.UpstreamRef = hmtraceRef
			manifest.SourceURL = "https://gitcode.com/diting/hmtrace/tree/" + hmtraceRef + "/assets/trace_streamer/" + map[PayloadExpectation]string{
				PayloadLinux:   "linux-x86_64",
				PayloadWindows: "windows-x86_64",
			}[contract.name]
		})
	}
	license := mustReadFile(t, filepath.Join(releaseArtifactRepositoryRoot(t), filepath.FromSlash(hmtraceLicenseFile)))
	if sha256Hex(license) != hmtraceLicenseSHA256 {
		t.Fatal("repository hmtrace license fixture hash drifted")
	}
	mustWriteFile(t, filepath.Join(repo, filepath.FromSlash(hmtraceLicenseFile)), license, 0o644)

	provenance := commercialProvenance{
		SchemaVersion: 1,
		Acquisition: provenanceAcquisition{
			Repository:              hmtraceRepository,
			Ref:                     hmtraceRef,
			RepositoryLicense:       "Apache-2.0",
			RepositoryLicenseFile:   hmtraceLicenseFile,
			RepositoryLicenseSHA256: hmtraceLicenseSHA256,
			Disclaimer:              hmtraceLicenseDisclaimer,
		},
		LicenseConcluded:     "NOASSERTION",
		RedistributionStatus: "blocked",
	}
	for _, contract := range platformContracts {
		payload := mustReadFile(t, payloadFile(repo, contract.name))
		minimumGlibc := ""
		if contract.goos == "linux" {
			minimumGlibc = "2.34"
		}
		provenance.Assets = append(provenance.Assets, provenanceAsset{
			GOOS:           contract.goos,
			GOARCH:         contract.goarch,
			SourcePath:     "assets/trace_streamer/" + map[PayloadExpectation]string{PayloadLinux: "linux-x86_64/trace_streamer", PayloadWindows: "windows-x86_64/trace_streamer.exe"}[contract.name],
			RepositoryPath: filepath.ToSlash(filepath.Join("internal", "hitraceconv", "embedded_trace_streamer", contract.directory, contract.binaryName)),
			SHA256:         sha256Hex(payload),
			SizeBytes:      int64(len(payload)),
			CopyMethod:     "verbatim_unmodified",
			MinimumGlibc:   minimumGlibc,
		})
	}
	if approved {
		provenance.LicenseConcluded = "LicenseRef-Test-Payload-Closure"
		provenance.RedistributionStatus = "approved"
		for _, contract := range platformContracts {
			mutateManifest(t, repo, contract.name, func(manifest *streamerManifest) {
				manifest.LicenseConcluded = provenance.LicenseConcluded
				manifest.RedistributionStatus = provenance.RedistributionStatus
			})
		}
		installCommercialEvidence(t, repo, &provenance)
	}
	writeCommercialProvenance(t, repo, provenance)
	return repo, provenance
}

func installCommercialLinuxPayloadFixture(t *testing.T, repo string) {
	t.Helper()
	sourceRepo := releaseArtifactRepositoryRoot(t)
	body := mustReadFile(t, payloadFile(sourceRepo, PayloadLinux))
	mustWriteFile(t, payloadFile(repo, PayloadLinux), body, 0o755)
	mutateManifest(t, repo, PayloadLinux, func(manifest *streamerManifest) {
		manifest.Platforms[0].SHA256 = sha256Hex(body)
		manifest.Platforms[0].SizeBytes = int64(len(body))
		manifest.Platforms[0].ActualFormat = "test fixture copied from the pinned repository Linux payload"
		manifest.Platforms[0].MinimumGlibc = "2.34"
	})
}

func installCommercialEvidence(t *testing.T, repo string, provenance *commercialProvenance) {
	t.Helper()
	writeEvidence := func(name, content string) *fileEvidence {
		path := "third_party/trace_streamer/evidence/" + name
		body := []byte(content)
		mustWriteFile(t, filepath.Join(repo, filepath.FromSlash(path)), body, 0o644)
		return &fileEvidence{Path: path, SHA256: sha256Hex(body)}
	}
	payloadHashes := make([]string, 0, len(provenance.Assets))
	for _, asset := range provenance.Assets {
		payloadHashes = append(payloadHashes, asset.SHA256)
	}
	bindings := commercialEvidenceBindings{
		SBOM:                    *writeEvidence("trace-streamer.spdx.json", `{"spdxVersion":"SPDX-2.3"}`),
		DependencyLicenseBundle: *writeEvidence("dependency-licenses.txt", "test dependency license closure"),
		ThirdPartyNotices:       *writeEvidence("THIRD_PARTY_NOTICES.txt", "test third-party notices"),
		SourceBuildAttestation:  *writeEvidence("source-build-attestation.json", `{"predicateType":"https://slsa.dev/provenance/v1"}`),
	}
	legal := &legalApprovalEvidence{
		Path:           "third_party/trace_streamer/evidence/legal-approval.json",
		ApprovalID:     "LEGAL-TEST-1",
		ApprovedBy:     "test-legal-authority",
		ApprovedAt:     "2026-07-14T00:00:00Z",
		PayloadSHA256s: payloadHashes,
	}
	document := legalApprovalDocument{
		SchemaVersion:    1,
		Decision:         "approved_for_commercial_redistribution",
		LicenseConcluded: provenance.LicenseConcluded,
		ApprovalID:       legal.ApprovalID,
		ApprovedBy:       legal.ApprovedBy,
		ApprovedAt:       legal.ApprovedAt,
		PayloadSHA256s:   payloadHashes,
		Evidence:         bindings,
	}
	legalBody := marshalJSON(t, document)
	mustWriteFile(t, filepath.Join(repo, filepath.FromSlash(legal.Path)), legalBody, 0o644)
	legal.SHA256 = sha256Hex(legalBody)
	provenance.CommercialReleaseEvidence = commercialReleaseEvidence{
		PayloadScopedLegalApproval: legal,
		SBOM:                       &bindings.SBOM,
		DependencyLicenseBundle:    &bindings.DependencyLicenseBundle,
		ThirdPartyNotices:          &bindings.ThirdPartyNotices,
		SourceBuildAttestation:     &bindings.SourceBuildAttestation,
	}
}

func rewriteLegalApprovalDocument(t *testing.T, repo string, provenance *commercialProvenance) {
	t.Helper()
	legal := provenance.CommercialReleaseEvidence.PayloadScopedLegalApproval
	document := legalApprovalDocument{
		SchemaVersion:    1,
		Decision:         "approved_for_commercial_redistribution",
		LicenseConcluded: provenance.LicenseConcluded,
		ApprovalID:       legal.ApprovalID,
		ApprovedBy:       legal.ApprovedBy,
		ApprovedAt:       legal.ApprovedAt,
		PayloadSHA256s:   legal.PayloadSHA256s,
		Evidence: commercialEvidenceBindings{
			SBOM:                    *provenance.CommercialReleaseEvidence.SBOM,
			DependencyLicenseBundle: *provenance.CommercialReleaseEvidence.DependencyLicenseBundle,
			ThirdPartyNotices:       *provenance.CommercialReleaseEvidence.ThirdPartyNotices,
			SourceBuildAttestation:  *provenance.CommercialReleaseEvidence.SourceBuildAttestation,
		},
	}
	body := marshalJSON(t, document)
	mustWriteFile(t, filepath.Join(repo, filepath.FromSlash(legal.Path)), body, 0o644)
	legal.SHA256 = sha256Hex(body)
}

func writeCommercialProvenance(t *testing.T, repo string, provenance commercialProvenance) {
	t.Helper()
	mustWriteFile(t, filepath.Join(repo, filepath.FromSlash(traceStreamerProvenancePath)), marshalJSON(t, provenance), 0o644)
}

func marshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(body, '\n')
}

func releaseArtifactRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate releaseartifact test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
}

func TestHmtraceRepositoryLicenseWitnessHash(t *testing.T) {
	path := filepath.Join(releaseArtifactRepositoryRoot(t), filepath.FromSlash(hmtraceLicenseFile))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256Hex(body); got != hmtraceLicenseSHA256 {
		t.Fatalf("hmtrace license sha256=%s want=%s", got, hmtraceLicenseSHA256)
	}
}

func TestHashBoundThirdPartyEvidenceDisablesGitTextConversion(t *testing.T) {
	path := filepath.Join(releaseArtifactRepositoryRoot(t), "third_party", ".gitattributes")
	body := string(mustReadFile(t, path))
	for _, contract := range []string{"hmtrace/** -text", "trace_streamer/** -text"} {
		if !strings.Contains(body, contract) {
			t.Fatalf("third-party byte-identity contract lacks %q", contract)
		}
	}
}
