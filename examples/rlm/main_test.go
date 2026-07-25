package main

import (
	"strings"
	"testing"
)

func TestBuildResourcesEmbeddedCorpus(t *testing.T) {
	resources, warnings, err := buildResources([]sourceSpec{
		{Title: "logs", Kind: "logs", Content: "first document"},
		{Title: "runbook", Kind: "runbook", Content: "local notes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(resources) != 2 {
		t.Fatalf("resources = %d, want 2", len(resources))
	}
	if resources[0].ID != "I01" || resources[0].Kind != "logs" || resources[0].Content != "first document" {
		t.Fatalf("first resource = %#v", resources[0])
	}
	if resources[1].ID != "I02" || resources[1].Kind != "runbook" || resources[1].Source == "" {
		t.Fatalf("second resource = %#v", resources[1])
	}
}

func TestBuildResourcesSkipsEmptyContent(t *testing.T) {
	resources, warnings, err := buildResources([]sourceSpec{
		{Title: "empty"},
		{Title: "logs", Content: "available"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].ID != "I02" {
		t.Fatalf("resources = %#v", resources)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "empty content") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestBuildResourcesEnforcesCorpusLimit(t *testing.T) {
	specs := make([]sourceSpec, 11)
	content := strings.Repeat("a", maxResourceBytes)
	for i := range specs {
		specs[i] = sourceSpec{Title: "document", Content: content}
	}

	resources, warnings, err := buildResources(specs)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, item := range resources {
		total += len(item.Content)
	}
	if total != maxCorpusBytes {
		t.Fatalf("corpus bytes = %d, want %d", total, maxCorpusBytes)
	}
	if len(resources) != 10 {
		t.Fatalf("resources = %d, want 10", len(resources))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "corpus size limit") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestBuildResourcesFailsWhenNothingLoads(t *testing.T) {
	resources, warnings, err := buildResources([]sourceSpec{
		{Title: "empty"},
	})
	if err == nil || !strings.Contains(err.Error(), "no resources") {
		t.Fatalf("error = %v", err)
	}
	if len(resources) != 0 || len(warnings) != 1 {
		t.Fatalf("resources=%v warnings=%v", resources, warnings)
	}
}

func TestLimitStringPreservesUTF8(t *testing.T) {
	value, truncated := limitString("абв", 3)
	if !truncated || value != "а" {
		t.Fatalf("value=%q truncated=%v", value, truncated)
	}
}

func TestEnvFallbackPrefersPrimary(t *testing.T) {
	t.Setenv("EXAMPLE_PRIMARY", "primary")
	t.Setenv("EXAMPLE_FALLBACK", "fallback")

	if got := envFallback("EXAMPLE_PRIMARY", "EXAMPLE_FALLBACK"); got != "primary" {
		t.Fatalf("envFallback = %q, want primary", got)
	}
}

func TestEnvFallbackUsesFallback(t *testing.T) {
	t.Setenv("EXAMPLE_FALLBACK", "fallback")

	if got := envFallback("EXAMPLE_PRIMARY", "EXAMPLE_FALLBACK"); got != "fallback" {
		t.Fatalf("envFallback = %q, want fallback", got)
	}
}
