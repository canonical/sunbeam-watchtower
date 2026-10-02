// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package bugsearch

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

func TestSearchTextRanksAndExplainsFields(t *testing.T) {
	service := NewService([]Document{
		testDocument("1", "Cinder snapshot interrupted", "The service restarted during an operation.", "snap-openstack"),
		testDocument("2", "Unrelated networking issue", "The API timed out.", "sunbeam"),
	})

	result, err := service.Search(dto.BugSearchRequest{Query: "cinder snapshot restart"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != dto.BugSearchOutcomeMatches || len(result.Results) != 1 {
		t.Fatalf("result = %+v, want one primary match", result)
	}
	got := result.Results[0]
	if got.ID != "1" || got.Classification != dto.BugSearchDirect {
		t.Fatalf("candidate = %+v, want direct bug 1", got)
	}
	if got.Score <= 0 || len(got.Evidence) < 2 {
		t.Fatalf("score/evidence = %f/%+v", got.Score, got.Evidence)
	}
	for _, evidence := range got.Evidence {
		if evidence.Excerpt == "" || evidence.Field == "" || len(evidence.Matches) == 0 ||
			evidence.Matches[0].MatchType == "" {
			t.Fatalf("incomplete evidence: %+v", evidence)
		}
	}
}

func TestSearchTextDistinguishesPartialAndRelated(t *testing.T) {
	service := NewService([]Document{
		testDocument("1", "Snapshot interrupted", "Cinder operation failed.", "snap-openstack"),
		testDocument("2", "Service restart race", "Nova service restarted.", "sunbeam"),
	})

	result, err := service.Search(dto.BugSearchRequest{
		Query: "cinder snapshot restart race",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("primary results = %d, want 2 partials: %+v", len(result.Results), result)
	}
	for _, candidate := range result.Results {
		if candidate.Classification != dto.BugSearchPartial {
			t.Fatalf("classification = %q, want partial", candidate.Classification)
		}
	}

	related, err := service.Search(dto.BugSearchRequest{
		Query: "microceph relation snapshot race condition",
	})
	if err != nil {
		t.Fatal(err)
	}
	if related.Outcome != dto.BugSearchOutcomeRelatedOnly || len(related.Related) == 0 {
		t.Fatalf("related response = %+v", related)
	}
}

func TestSearchTextConstraintsAndStrictLexicalMode(t *testing.T) {
	service := NewService([]Document{
		testDocument("1", "Services restarting quickly", "relation-changed fired", "snap-openstack"),
		testDocument("2", "Service restart without HA", "noha deployment", "snap-openstack"),
	})
	fuzzy := true
	result, err := service.Search(dto.BugSearchRequest{
		Query:         "restart",
		Phrases:       []string{"relation changed"},
		RequiredTerms: []string{"service"},
		ExcludedTerms: []string{"noha"},
		Fuzzy:         &fuzzy,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0].ID != "1" {
		t.Fatalf("result = %+v, want constrained bug 1", result)
	}

	strict := false
	result, err = service.Search(dto.BugSearchRequest{Query: "restart", Fuzzy: &strict})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0].ID != "2" {
		t.Fatalf("strict result = %+v, want only exact restart", result)
	}
}

func TestSearchRegexValidationFieldsAndCase(t *testing.T) {
	service := NewService([]Document{
		testDocument("1", "CINDER restart", "description", "snap-openstack"),
	})
	_, err := service.Search(dto.BugSearchRequest{Mode: "regex", Query: "["})
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("error = %v, want ErrInvalidQuery", err)
	}

	result, err := service.Search(dto.BugSearchRequest{
		Mode:   "regex",
		Query:  `cinder\s+restart`,
		Fields: []string{"title"},
	})
	if err != nil || len(result.Results) != 1 {
		t.Fatalf("case-insensitive regex = %+v, %v", result, err)
	}

	result, err = service.Search(dto.BugSearchRequest{
		Mode:          "regex",
		Query:         `cinder`,
		Fields:        []string{"title"},
		CaseSensitive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != dto.BugSearchOutcomeNoResults {
		t.Fatalf("case-sensitive outcome = %q, want no_results", result.Outcome)
	}
}

func TestSearchMergingAndTaskFilters(t *testing.T) {
	document := testDocument("1", "Cinder snapshot race", "description", "project-a")
	second := document.Tasks[0]
	second.Project = "project-b"
	second.Status = "Fix Released"
	second.TargetName = "target-b"
	document.Tasks = append(document.Tasks, second)
	service := NewService([]Document{document})

	result, err := service.Search(dto.BugSearchRequest{
		Query:    "snapshot",
		Projects: []string{"project-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || len(result.Results[0].Tasks) != 2 ||
		len(result.Results[0].MatchedTasks) != 1 {
		t.Fatalf("merged result = %+v", result)
	}

	merge := false
	result, err = service.Search(dto.BugSearchRequest{Query: "snapshot", Merge: &merge})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("non-merged results = %d, want 2", len(result.Results))
	}

	result, err = service.Search(dto.BugSearchRequest{
		Query:  "snapshot",
		Closed: "exclude",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || len(result.Results[0].MatchedTasks) != 1 {
		t.Fatalf("open-only result = %+v", result)
	}
}

func TestSearchExcerptsReportTruncation(t *testing.T) {
	prefix := strings.Repeat("before ", 30)
	suffix := strings.Repeat(" after", 30)
	service := NewService([]Document{
		testDocument("1", "title", prefix+"snapshot"+suffix, "project"),
	})
	result, err := service.Search(dto.BugSearchRequest{Query: "snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	evidence := result.Results[0].Evidence[0]
	if !evidence.TruncatedBefore || !evidence.TruncatedAfter {
		t.Fatalf("truncation flags = %+v", evidence)
	}
	if !strings.Contains(evidence.Excerpt, "snapshot") {
		t.Fatalf("excerpt %q omits match", evidence.Excerpt)
	}
}

func TestSearchErrorsForEmptyCacheAndQuery(t *testing.T) {
	_, err := NewService(nil).Search(dto.BugSearchRequest{Query: "snapshot"})
	if !errors.Is(err, ErrCacheEmpty) {
		t.Fatalf("error = %v, want ErrCacheEmpty", err)
	}
	_, err = NewService([]Document{testDocument("1", "title", "description", "project")}).
		Search(dto.BugSearchRequest{ExcludedTerms: []string{"noise"}})
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("error = %v, want ErrInvalidQuery", err)
	}
	_, err = NewService([]Document{testDocument("1", "title", "description", "project")}).
		Search(dto.BugSearchRequest{
			Query:         "snapshot",
			EvidenceLimit: MaxEvidenceLimit + 1,
		})
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("evidence limit error = %v, want ErrInvalidQuery", err)
	}
	_, err = NewService([]Document{testDocument("1", "title", "description", "project")}).
		Search(dto.BugSearchRequest{Query: strings.Repeat("x", MaxQueryBytes+1)})
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("query size error = %v, want ErrInvalidQuery", err)
	}
}

func TestDirectReference(t *testing.T) {
	tests := map[string]string{
		"123":           "launchpad:123",
		"launchpad:456": "launchpad:456",
		"https://bugs.launchpad.net/sunbeam/+bug/789":        "launchpad:789",
		"https://bugs.launchpad.net/ubuntu/+source/x/+bug/9": "launchpad:9",
		"https://launchpad.net/bugs/10":                      "launchpad:10",
	}
	for input, want := range tests {
		got, ok := DirectReference(input)
		if !ok || got != want {
			t.Errorf("DirectReference(%q) = %q, %t; want %q, true", input, got, ok, want)
		}
	}
	if _, ok := DirectReference("snapshot 123"); ok {
		t.Fatal("sentence containing digits must not become an identifier search")
	}
}

func TestSearchDirectReferenceUsesSnapshot(t *testing.T) {
	service := NewService([]Document{
		testDocument("2161602", "Cached exact bug", "cached description", "cinder"),
	})
	result, err := service.Search(dto.BugSearchRequest{Query: "launchpad:2161602"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0].ID != "2161602" ||
		result.Results[0].Evidence[0].Field != "id" {
		t.Fatalf("direct reference result = %+v", result)
	}

	missing, err := service.Search(dto.BugSearchRequest{Query: "launchpad:9999999"})
	if err != nil {
		t.Fatal(err)
	}
	if missing.Outcome != dto.BugSearchOutcomeNoResults {
		t.Fatalf("missing direct reference outcome = %q", missing.Outcome)
	}
}

func TestCommentVolumeDoesNotMultiplyConceptScore(t *testing.T) {
	closer := testDocument(
		"1",
		"MicroCeph monitor address reordering restarts cinder-volume",
		"An in-flight snapshot is interrupted.",
		"cinder",
	)
	verbose := testDocument("2", "Generic storage discussion", "No specific diagnosis.", "cinder")
	for i := 0; i < 100; i++ {
		verbose.Bug.Comments = append(verbose.Bug.Comments, forge.BugComment{
			Author: fmt.Sprintf("user-%d", i),
			Body:   "Someone mentioned restart and snapshot.",
		})
	}
	result, err := NewService([]Document{verbose, closer}).Search(dto.BugSearchRequest{
		Query: "microceph cinder-volume monitor address restart snapshot",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) == 0 || result.Results[0].ID != "1" {
		t.Fatalf("ranked results = %+v, want closer title match first", result.Results)
	}
}

func TestCinderMicroCephIncidentCorpusIsDirectAndBounded(t *testing.T) {
	const incident = "A MicroCeph update-status republishes the same monitor addresses " +
		"in a different order, triggers relation-changed on every cinder-volume-ceph unit, " +
		"restarts all cinder-volume services, and loses an in-flight snapshot."

	documents := make([]Document, 0, 1058)
	exact := testDocument(
		"2161602",
		"MicroCeph monitor address reordering restarts all cinder-volume services",
		"update-status republishes monitor addresses in a different order and triggers "+
			"relation-changed on cinder-volume-ceph; an in-flight snapshot is lost.",
		"cinder",
	)
	documents = append(documents, exact)
	for i := 1; i < 1058; i++ {
		document := testDocument(
			fmt.Sprintf("%d", 3000000+i),
			fmt.Sprintf("Generic service restart report %d", i),
			"A storage operation may be interrupted.",
			"cinder",
		)
		for comment := 0; comment < 5; comment++ {
			document.Bug.Comments = append(document.Bug.Comments, forge.BugComment{
				Author: "reporter",
				Body:   "The service restart was discussed again.",
			})
		}
		documents = append(documents, document)
	}

	start := time.Now()
	result, err := NewService(documents).Search(dto.BugSearchRequest{Query: incident})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("incident search took %s, want <= 2s", elapsed)
	}
	if len(result.Results) == 0 || result.Results[0].ID != "2161602" ||
		result.Results[0].Classification != dto.BugSearchDirect {
		t.Fatalf("top result = %+v, want direct launchpad:2161602", result.Results)
	}
	if len(result.Results) > DefaultLimit || len(result.Related) > DefaultRelatedLimit {
		t.Fatalf("result bounds = %d primary, %d related", len(result.Results), len(result.Related))
	}
	for _, group := range [][]dto.BugSearchResult{result.Results, result.Related} {
		for _, candidate := range group {
			if len(candidate.Evidence) > DefaultEvidenceLimit {
				t.Fatalf("%s evidence count = %d", candidate.Reference, len(candidate.Evidence))
			}
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 200_000 {
		t.Fatalf("JSON response is %d bytes, want <= 200000", len(encoded))
	}
}

func testDocument(id, title, description, project string) Document {
	now := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	return Document{
		Bug: &forge.Bug{
			Forge:           forge.ForgeLaunchpad,
			ID:              id,
			Title:           title,
			Description:     description,
			URL:             "https://bugs.launchpad.net/sunbeam/+bug/" + id,
			VisibilityKnown: true,
			CreatedAt:       now.Add(-24 * time.Hour),
			UpdatedAt:       now,
		},
		Tasks: []forge.BugTask{{
			Forge:           forge.ForgeLaunchpad,
			Project:         project,
			BugID:           id,
			Title:           title,
			Status:          "New",
			Importance:      "High",
			TargetName:      "target-a",
			VisibilityKnown: true,
			CreatedAt:       now.Add(-24 * time.Hour),
			UpdatedAt:       now,
		}},
		Provenance: []dto.BugSearchSource{{
			Forge:    "launchpad",
			Project:  project,
			Source:   "cache",
			SyncedAt: now,
		}},
	}
}
