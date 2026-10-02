// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

// Package bugsearch implements bounded, explainable search over complete
// cached bug documents. Classifications describe query evidence coverage; they
// do not assert that two incidents have the same cause.
package bugsearch

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

const (
	DefaultLimit         = 10
	DefaultRelatedLimit  = 5
	DefaultEvidenceLimit = 5
	MaxLimit             = 100
	MaxRelatedLimit      = 50
	MaxEvidenceLimit     = 20
	MaxQueryBytes        = 4096
	MaxConstraintEntries = 32
	MaxConstraintBytes   = 256
	MaxConcepts          = 64
	ScoringVersion       = "explainable-v2"
)

var (
	// ErrInvalidQuery marks user-correctable query and filter errors.
	ErrInvalidQuery = errors.New("invalid bug search query")
	// ErrCacheEmpty means text search has no complete cached documents.
	ErrCacheEmpty = errors.New("bug search cache is empty")
	// ErrCacheIncompatible means a full bug cache refresh is required.
	ErrCacheIncompatible = errors.New("bug search cache schema is incompatible")
)

var defaultFields = []string{"title", "description", "comments", "tags"}

var supportedFields = map[string]float64{
	"title":       8,
	"description": 5,
	"comments":    3,
	"tags":        2,
	"project":     1,
	"tracker":     1,
	"status":      1,
	"importance":  1,
	"assignee":    1,
	"tasks":       1,
	"id":          20,
	"url":         20,
}

var closedStatuses = map[string]bool{
	"fix released":   true,
	"invalid":        true,
	"won't fix":      true,
	"expired":        true,
	"does not exist": true,
}

// Document is one complete bug plus Watchtower-project-expanded tasks.
type Document struct {
	Bug        *forge.Bug
	Tasks      []forge.BugTask
	Provenance []dto.BugSearchSource
}

// Service searches an immutable snapshot. Construct one per request so config
// reloads and cache syncs are observed immediately.
type Service struct {
	documents []Document
}

// NewService creates a search service over one cache snapshot.
func NewService(documents []Document) *Service {
	return &Service{documents: documents}
}

// Search validates and evaluates one query.
func (s *Service) Search(req dto.BugSearchRequest) (*dto.BugSearchResponse, error) {
	resolved, err := resolveRequest(req)
	if err != nil {
		return nil, err
	}
	if len(s.documents) == 0 {
		return nil, ErrCacheEmpty
	}

	candidates := make([]candidate, 0, len(s.documents))
	for _, document := range s.documents {
		filteredTasks := matchingTasks(document.Tasks, resolved)
		if taskFiltersActive(resolved) && len(filteredTasks) == 0 {
			continue
		}
		if !trackerMatches(document.Bug, resolved.trackers) {
			continue
		}
		candidates = append(candidates, candidate{
			document:     document,
			matchedTasks: filteredTasks,
		})
	}

	if len(candidates) == 0 {
		return emptyResponse(resolved, len(s.documents)), nil
	}

	if resolved.mode == string(dto.BugSearchModeRegex) {
		return searchRegex(candidates, resolved), nil
	}
	if reference, ok := DirectReference(req.Query); ok &&
		len(resolved.phrases) == 0 &&
		len(resolved.required) == 0 &&
		len(resolved.excluded) == 0 {
		return searchDirectReference(candidates, resolved, reference), nil
	}
	return searchText(candidates, resolved), nil
}

type resolvedRequest struct {
	dto.BugSearchRequest
	mode           string
	fields         []string
	fuzzy          bool
	merge          bool
	closed         string
	sort           string
	limit          int
	relatedLimit   int
	evidenceLimit  int
	createdAfter   *time.Time
	createdBefore  *time.Time
	modifiedAfter  *time.Time
	modifiedBefore *time.Time
	trackers       map[string]bool
	concepts       []concept
	required       []concept
	phrases        []string
	excluded       []string
	regex          *regexp.Regexp
}

func resolveRequest(req dto.BugSearchRequest) (resolvedRequest, error) {
	resolved := resolvedRequest{BugSearchRequest: req}
	if len(req.Query) > MaxQueryBytes {
		return resolved, invalidf("query must not exceed %d bytes", MaxQueryBytes)
	}
	for name, values := range map[string][]string{
		"phrases": req.Phrases, "required_terms": req.RequiredTerms,
		"excluded_terms": req.ExcludedTerms,
	} {
		if len(values) > MaxConstraintEntries {
			return resolved, invalidf("%s must contain at most %d entries", name, MaxConstraintEntries)
		}
		for _, value := range values {
			if len(value) > MaxConstraintBytes {
				return resolved, invalidf("%s entries must not exceed %d bytes", name, MaxConstraintBytes)
			}
		}
	}
	resolved.mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if resolved.mode == "" {
		resolved.mode = string(dto.BugSearchModeText)
	}
	if resolved.mode != string(dto.BugSearchModeText) && resolved.mode != string(dto.BugSearchModeRegex) {
		return resolved, invalidf("mode must be text or regex")
	}

	resolved.fields = append([]string(nil), req.Fields...)
	if len(resolved.fields) == 0 {
		resolved.fields = append([]string(nil), defaultFields...)
	}
	seenFields := make(map[string]bool)
	for i, field := range resolved.fields {
		field = strings.ToLower(strings.TrimSpace(field))
		if _, ok := supportedFields[field]; !ok {
			return resolved, invalidf("unsupported field %q", field)
		}
		if seenFields[field] {
			return resolved, invalidf("duplicate field %q", field)
		}
		seenFields[field] = true
		resolved.fields[i] = field
	}

	resolved.fuzzy = true
	if req.Fuzzy != nil {
		resolved.fuzzy = *req.Fuzzy
	}
	resolved.merge = true
	if req.Merge != nil {
		resolved.merge = *req.Merge
	}
	resolved.closed = strings.ToLower(strings.TrimSpace(req.Closed))
	if resolved.closed == "" {
		resolved.closed = "include"
	}
	if !slices.Contains([]string{"include", "exclude", "only"}, resolved.closed) {
		return resolved, invalidf("closed must be include, exclude, or only")
	}
	resolved.sort = strings.ToLower(strings.TrimSpace(req.Sort))
	if resolved.sort == "" {
		resolved.sort = "relevance"
	}
	if !slices.Contains([]string{"relevance", "modified", "created", "importance", "status"}, resolved.sort) {
		return resolved, invalidf("unsupported sort %q", resolved.sort)
	}
	resolved.limit = req.Limit
	if resolved.limit == 0 {
		resolved.limit = DefaultLimit
	}
	if req.RelatedLimit == nil {
		resolved.relatedLimit = DefaultRelatedLimit
	} else {
		resolved.relatedLimit = *req.RelatedLimit
	}
	if resolved.limit < 1 || resolved.limit > MaxLimit {
		return resolved, invalidf("limit must be between 1 and %d", MaxLimit)
	}
	if resolved.relatedLimit < 0 || resolved.relatedLimit > MaxRelatedLimit {
		return resolved, invalidf("related limit must be between 0 and %d", MaxRelatedLimit)
	}
	resolved.evidenceLimit = req.EvidenceLimit
	if resolved.evidenceLimit == 0 {
		resolved.evidenceLimit = DefaultEvidenceLimit
	}
	if resolved.evidenceLimit < 1 || resolved.evidenceLimit > MaxEvidenceLimit {
		return resolved, invalidf("evidence limit must be between 1 and %d", MaxEvidenceLimit)
	}

	var err error
	if resolved.createdAfter, err = parseBoundary(req.CreatedAfter); err != nil {
		return resolved, invalidf("invalid created_after: %v", err)
	}
	if resolved.createdBefore, err = parseBoundary(req.CreatedBefore); err != nil {
		return resolved, invalidf("invalid created_before: %v", err)
	}
	if resolved.modifiedAfter, err = parseBoundary(req.ModifiedAfter); err != nil {
		return resolved, invalidf("invalid modified_after: %v", err)
	}
	if resolved.modifiedBefore, err = parseBoundary(req.ModifiedBefore); err != nil {
		return resolved, invalidf("invalid modified_before: %v", err)
	}
	if invalidRange(resolved.createdAfter, resolved.createdBefore) {
		return resolved, invalidf("created_after must be before created_before")
	}
	if invalidRange(resolved.modifiedAfter, resolved.modifiedBefore) {
		return resolved, invalidf("modified_after must be before modified_before")
	}

	resolved.trackers = stringSet(req.Trackers)
	resolved.phrases = cleanNonEmpty(req.Phrases)
	resolved.excluded = normalizedTerms(req.ExcludedTerms, false)

	query := strings.TrimSpace(req.Query)
	if resolved.mode == string(dto.BugSearchModeRegex) {
		if query == "" {
			return resolved, invalidf("regex mode requires a query")
		}
		if len(resolved.phrases) > 0 || len(req.RequiredTerms) > 0 || len(req.ExcludedTerms) > 0 {
			return resolved, invalidf("regex mode cannot be combined with phrase, require, or exclude constraints")
		}
		if req.Fuzzy != nil {
			return resolved, invalidf("regex mode cannot set fuzzy matching")
		}
		pattern := query
		if !req.CaseSensitive {
			pattern = "(?i)" + pattern
		}
		resolved.regex, err = regexp.Compile(pattern)
		if err != nil {
			return resolved, invalidf("invalid regular expression: %v", err)
		}
		return resolved, nil
	}
	if req.CaseSensitive {
		return resolved, invalidf("case_sensitive is only valid in regex mode")
	}

	resolved.concepts = conceptsFromText(query, resolved.fuzzy)
	resolved.required = conceptsFromTerms(req.RequiredTerms, resolved.fuzzy)
	if len(resolved.concepts)+len(resolved.required)+len(resolved.phrases) > MaxConcepts {
		return resolved, invalidf("query must contain at most %d concepts", MaxConcepts)
	}
	if len(resolved.concepts) == 0 && len(resolved.required) == 0 && len(resolved.phrases) == 0 {
		return resolved, invalidf("text mode requires a query, phrase, or required term")
	}
	return resolved, nil
}

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidQuery, fmt.Sprintf(format, args...))
}

func parseBoundary(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		t = t.UTC()
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, fmt.Errorf("expected RFC 3339 timestamp or YYYY-MM-DD")
	}
	t = t.UTC()
	return &t, nil
}

func invalidRange(after, before *time.Time) bool {
	return after != nil && before != nil && !after.Before(*before)
}

func cleanNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			set[value] = true
		}
	}
	return set
}

func trackerMatches(bug *forge.Bug, trackers map[string]bool) bool {
	if len(trackers) == 0 {
		return true
	}
	return trackers[strings.ToLower(bug.Forge.String())]
}

func taskFiltersActive(req resolvedRequest) bool {
	return len(req.Projects) > 0 ||
		len(req.Status) > 0 ||
		len(req.Importance) > 0 ||
		req.Assignee != "" ||
		len(req.Tags) > 0 ||
		req.closed != "include" ||
		req.createdAfter != nil ||
		req.createdBefore != nil ||
		req.modifiedAfter != nil ||
		req.modifiedBefore != nil
}

func matchingTasks(tasks []forge.BugTask, req resolvedRequest) []forge.BugTask {
	projects := stringSet(req.Projects)
	statuses := stringSet(req.Status)
	importances := stringSet(req.Importance)
	tags := stringSet(req.Tags)
	assignee := strings.ToLower(strings.TrimSpace(req.Assignee))
	result := make([]forge.BugTask, 0, len(tasks))
	for _, task := range tasks {
		if len(projects) > 0 && !projects[strings.ToLower(task.Project)] {
			continue
		}
		if len(statuses) > 0 && !statuses[strings.ToLower(task.Status)] {
			continue
		}
		if len(importances) > 0 && !importances[strings.ToLower(task.Importance)] {
			continue
		}
		if assignee != "" && strings.ToLower(task.Assignee) != assignee {
			continue
		}
		if !containsAllFold(task.Tags, tags) {
			continue
		}
		closed := closedStatuses[strings.ToLower(task.Status)]
		if req.closed == "exclude" && closed {
			continue
		}
		if req.closed == "only" && !closed {
			continue
		}
		if req.createdAfter != nil && task.CreatedAt.Before(*req.createdAfter) {
			continue
		}
		if req.createdBefore != nil && !task.CreatedAt.Before(*req.createdBefore) {
			continue
		}
		if req.modifiedAfter != nil && task.UpdatedAt.Before(*req.modifiedAfter) {
			continue
		}
		if req.modifiedBefore != nil && !task.UpdatedAt.Before(*req.modifiedBefore) {
			continue
		}
		result = append(result, task)
	}
	if !taskFiltersActive(req) {
		return append([]forge.BugTask(nil), tasks...)
	}
	return result
}

func containsAllFold(values []string, required map[string]bool) bool {
	if len(required) == 0 {
		return true
	}
	found := make(map[string]bool, len(values))
	for _, value := range values {
		found[strings.ToLower(value)] = true
	}
	for value := range required {
		if !found[value] {
			return false
		}
	}
	return true
}

type candidate struct {
	document     Document
	matchedTasks []forge.BugTask
}

func searchText(candidates []candidate, req resolvedRequest) *dto.BugSearchResponse {
	indexed := indexCandidates(candidates, req)
	df := conceptDocumentFrequency(indexed, req)
	var primary []dto.BugSearchResult
	var related []dto.BugSearchResult
	for _, item := range indexed {
		result, matched := evaluateText(item, req, df, len(indexed))
		if !matched {
			continue
		}
		for _, expanded := range expandTaskScopedResults(result, item.candidate, req.merge) {
			if expanded.Classification == dto.BugSearchRelated {
				related = append(related, expanded)
			} else {
				primary = append(primary, expanded)
			}
		}
	}
	return finishResponse(primary, related, req, len(candidates))
}

func searchRegex(candidates []candidate, req resolvedRequest) *dto.BugSearchResponse {
	indexed := indexCandidates(candidates, req)
	var primary []dto.BugSearchResult
	var related []dto.BugSearchResult
	for _, item := range indexed {
		result, matched := evaluateRegex(item, req)
		if !matched {
			continue
		}
		for _, expanded := range expandTaskScopedResults(result, item.candidate, req.merge) {
			if expanded.Classification == dto.BugSearchRelated {
				related = append(related, expanded)
			} else {
				primary = append(primary, expanded)
			}
		}
	}
	return finishResponse(primary, related, req, len(candidates))
}

func finishResponse(primary, related []dto.BugSearchResult, req resolvedRequest, documentCount int) *dto.BugSearchResponse {
	sortResults(primary, req.sort)
	sortResults(related, req.sort)
	totalPrimary := len(primary)
	totalRelated := len(related)
	truncated := totalPrimary > req.limit || totalRelated > req.relatedLimit
	if len(primary) > req.limit {
		primary = primary[:req.limit]
	}
	if len(related) > req.relatedLimit {
		related = related[:req.relatedLimit]
	}
	outcome := dto.BugSearchOutcomeNoResults
	if totalPrimary > 0 {
		outcome = dto.BugSearchOutcomeMatches
	} else if totalRelated > 0 {
		outcome = dto.BugSearchOutcomeRelatedOnly
	}
	return &dto.BugSearchResponse{
		Outcome:        outcome,
		Results:        primary,
		Related:        related,
		TotalResults:   totalPrimary,
		TotalRelated:   totalRelated,
		Truncated:      truncated,
		Applied:        applied(req, documentCount),
		Provenance:     collectProvenance(primary, related),
		ScoringVersion: ScoringVersion,
	}
}

func emptyResponse(req resolvedRequest, documentCount int) *dto.BugSearchResponse {
	return &dto.BugSearchResponse{
		Outcome:        dto.BugSearchOutcomeNoResults,
		Results:        []dto.BugSearchResult{},
		Related:        []dto.BugSearchResult{},
		Applied:        applied(req, documentCount),
		ScoringVersion: ScoringVersion,
	}
}

func applied(req resolvedRequest, documentCount int) dto.BugSearchApplied {
	return dto.BugSearchApplied{
		Mode:          req.mode,
		Fields:        append([]string(nil), req.fields...),
		Fuzzy:         req.fuzzy,
		Closed:        req.closed,
		Merge:         req.merge,
		Sort:          req.sort,
		Limit:         req.limit,
		RelatedLimit:  req.relatedLimit,
		EvidenceLimit: req.evidenceLimit,
		DocumentCount: documentCount,
	}
}

func collectProvenance(groups ...[]dto.BugSearchResult) []dto.BugSearchSource {
	seen := make(map[string]bool)
	var result []dto.BugSearchSource
	for _, group := range groups {
		for _, candidate := range group {
			for _, source := range candidate.Provenance {
				key := source.Forge + "\x00" + source.Project + "\x00" + source.Source + "\x00" + source.SyncedAt.String()
				if !seen[key] {
					seen[key] = true
					result = append(result, source)
				}
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Forge != result[j].Forge {
			return result[i].Forge < result[j].Forge
		}
		return result[i].Project < result[j].Project
	})
	return result
}

func sortResults(results []dto.BugSearchResult, sortBy string) {
	sort.SliceStable(results, func(i, j int) bool {
		left, right := results[i], results[j]
		switch sortBy {
		case "modified":
			if !left.UpdatedAt.Equal(right.UpdatedAt) {
				return left.UpdatedAt.After(right.UpdatedAt)
			}
		case "created":
			if !left.CreatedAt.Equal(right.CreatedAt) {
				return left.CreatedAt.After(right.CreatedAt)
			}
		case "importance":
			if importanceRank(left.Importance) != importanceRank(right.Importance) {
				return importanceRank(left.Importance) < importanceRank(right.Importance)
			}
		case "status":
			leftStatus := strings.Join(left.Status, ",")
			rightStatus := strings.Join(right.Status, ",")
			if leftStatus != rightStatus {
				return leftStatus < rightStatus
			}
		default:
			if classRank(left.Classification) != classRank(right.Classification) {
				return classRank(left.Classification) < classRank(right.Classification)
			}
			if left.Score != right.Score {
				return left.Score > right.Score
			}
			if !left.UpdatedAt.Equal(right.UpdatedAt) {
				return left.UpdatedAt.After(right.UpdatedAt)
			}
		}
		return left.Reference < right.Reference
	})
}

func classRank(classification dto.BugSearchClassification) int {
	switch classification {
	case dto.BugSearchDirect:
		return 0
	case dto.BugSearchPartial:
		return 1
	default:
		return 2
	}
}

func importanceRank(values []string) int {
	order := map[string]int{
		"critical":  0,
		"high":      1,
		"medium":    2,
		"low":       3,
		"wishlist":  4,
		"undecided": 5,
	}
	best := 99
	for _, value := range values {
		if rank, ok := order[strings.ToLower(value)]; ok && rank < best {
			best = rank
		}
	}
	return best
}

func resultBase(item candidate) dto.BugSearchResult {
	bug := item.document.Bug
	tasks := item.document.Tasks
	return dto.BugSearchResult{
		Reference:       strings.ToLower(bug.Forge.String()) + ":" + bug.ID,
		Forge:           bug.Forge,
		ID:              bug.ID,
		Title:           bug.Title,
		URL:             bug.URL,
		Projects:        uniqueTaskValues(tasks, func(task forge.BugTask) string { return task.Project }),
		Status:          uniqueTaskValues(tasks, func(task forge.BugTask) string { return task.Status }),
		Importance:      uniqueTaskValues(tasks, func(task forge.BugTask) string { return task.Importance }),
		Tags:            append([]string(nil), bug.Tags...),
		CreatedAt:       bug.CreatedAt,
		UpdatedAt:       maxUpdatedAt(bug, tasks),
		Tasks:           append([]forge.BugTask(nil), tasks...),
		MatchedTasks:    taskReferences(item.matchedTasks),
		Private:         bug.Private,
		SecurityRelated: bug.SecurityRelated,
		InformationType: bug.InformationType,
		Provenance:      append([]dto.BugSearchSource(nil), item.document.Provenance...),
	}
}

func uniqueTaskValues(tasks []forge.BugTask, value func(forge.BugTask) string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, task := range tasks {
		v := strings.TrimSpace(value(task))
		if v != "" && !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	sort.Strings(result)
	return result
}

func taskReferences(tasks []forge.BugTask) []string {
	refs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		refs = append(refs, taskReference(task))
	}
	sort.Strings(refs)
	return refs
}

func maxUpdatedAt(bug *forge.Bug, tasks []forge.BugTask) time.Time {
	updated := bug.UpdatedAt
	for _, task := range tasks {
		if task.UpdatedAt.After(updated) {
			updated = task.UpdatedAt
		}
	}
	return updated
}

// DirectReference recognizes only unambiguous Launchpad identifiers and URLs.
// It returns a canonical reference suitable for bug show.
func DirectReference(query string) (string, bool) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", false
	}
	if _, err := strconv.Atoi(query); err == nil {
		return "launchpad:" + query, true
	}
	lower := strings.ToLower(query)
	if strings.HasPrefix(lower, "launchpad:") {
		id := strings.TrimSpace(query[len("launchpad:"):])
		if _, err := strconv.Atoi(id); err == nil {
			return "launchpad:" + id, true
		}
	}
	re := regexp.MustCompile(`(?i)^https?://(?:bugs\.)?launchpad\.net/(?:bugs/|.*/\+bug/)(\d+)(?:[/?#].*)?$`)
	if match := re.FindStringSubmatch(query); len(match) == 2 {
		return "launchpad:" + match[1], true
	}
	return "", false
}

func SplitReference(reference string) (forge.ForgeType, string, error) {
	reference = strings.TrimSpace(reference)
	if canonical, ok := DirectReference(reference); ok {
		reference = canonical
	}
	parts := strings.SplitN(reference, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return 0, "", invalidf("bug reference must be <forge>:<id>")
	}
	switch strings.ToLower(parts[0]) {
	case "launchpad":
		return forge.ForgeLaunchpad, parts[1], nil
	default:
		return 0, "", invalidf("unsupported bug forge %q", parts[0])
	}
}
