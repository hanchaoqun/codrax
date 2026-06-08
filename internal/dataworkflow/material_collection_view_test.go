package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestBuildMaterialCollectionViewsIncludesRelatedTextAndDirectories(t *testing.T) {
	views := BuildMaterialCollectionViews([]dataquery.DataArtifact{{
		ID:   "inventory",
		Kind: string(dataquery.DataActionMaterialInventory),
		Children: []dataquery.DataArtifact{
			{
				ID:          "evidence/a.txt",
				Kind:        "text",
				SourcePaths: []string{"evidence/a.txt"},
			},
			{
				ID:          "evidence/b.txt",
				Kind:        "text",
				SourcePaths: []string{"evidence/b.txt"},
			},
			{
				ID:          "scan/a.png",
				Kind:        "image",
				SourcePaths: []string{"scan/a.png"},
				Fields:      map[string]string{"text_evidence_paths": "evidence/a.txt, evidence/b.txt"},
			},
		},
	}}, 8)
	if len(views) < 2 {
		t.Fatalf("views=%+v, want related text and directory handles", views)
	}
	var sawRelated, sawDir bool
	for _, handle := range views {
		if handle.Kind == "related_text_evidence" && strings.Join(handle.TextEvidencePaths, ",") == "evidence/a.txt,evidence/b.txt" {
			sawRelated = true
		}
		if handle.ID == "dir:evidence" && strings.Join(handle.MemberPaths, ",") == "evidence/a.txt,evidence/b.txt" {
			sawDir = true
		}
	}
	if !sawRelated || !sawDir {
		t.Fatalf("views=%+v, want related=%t dir=%t", views, sawRelated, sawDir)
	}
}

func TestBuildMaterialCollectionViewsBoundsAndOrdersRelatedFirst(t *testing.T) {
	views := BuildMaterialCollectionViews([]dataquery.DataArtifact{
		{
			ID:          "source/b.png",
			Kind:        "image",
			SourcePaths: []string{"source/b.png"},
			Fields:      map[string]string{"text_evidence_paths": "text/b.txt"},
		},
		{
			ID:          "source/a.png",
			Kind:        "image",
			SourcePaths: []string{"source/a.png"},
			Fields:      map[string]string{"text_evidence_paths": "text/a.txt"},
		},
		{
			ID:          "group/one.txt",
			Kind:        "text",
			SourcePaths: []string{"group/one.txt"},
		},
		{
			ID:          "group/two.txt",
			Kind:        "text",
			SourcePaths: []string{"group/two.txt"},
		},
	}, 1)
	if len(views) != 1 {
		t.Fatalf("view count=%d, want 1", len(views))
	}
	if views[0].ID != "related_text:source/a.png" {
		t.Fatalf("first view=%+v, want stable related text order before directory groups", views[0])
	}
}
