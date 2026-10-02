// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package bugsearch

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kljensen/snowball"
	"golang.org/x/text/unicode/norm"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	forge "github.com/canonical/sunbeam-watchtower/pkg/forge/v1"
)

var (
	wordPattern   = regexp.MustCompile(`[\pL\pN]+`)
	clausePattern = regexp.MustCompile(`(?i)\b(?:and|but|while|then|when|because)\b|[,.:;!?]+`)
)

var stopWords = map[string]bool{
	"a": true, "all": true, "also": true, "an": true, "and": true,
	"any": true, "are": true, "as": true, "at": true, "be": true,
	"been": true, "being": true, "both": true, "by": true, "can": true,
	"could": true, "did": true, "different": true, "do": true, "does": true,
	"each": true, "either": true, "every": true, "for": true, "from": true,
	"had": true, "has": true, "have": true, "here": true, "how": true,
	"if": true, "in": true, "into": true, "is": true, "it": true,
	"its": true, "may": true, "more": true, "most": true, "no": true,
	"not": true, "of": true, "on": true, "one": true, "only": true,
	"or": true, "other": true, "our": true, "same": true, "should": true,
	"some": true, "such": true, "than": true, "that": true, "the": true,
	"their": true, "them": true, "then": true, "there": true, "these": true,
	"they": true, "this": true, "those": true, "to": true, "too": true,
	"under": true, "up": true, "us": true, "was": true, "we": true,
	"were": true, "what": true, "when": true, "where": true, "which": true,
	"while": true, "who": true, "will": true, "with": true, "would": true,
	"you": true, "your": true,
}

type aliasDefinition struct {
	canonical string
	variants  [][]string
}

// Naming aliases are deliberately narrow: they normalize product and hook
// names without asserting that differently worded incidents have the same
// cause.
var aliasDefinitions = []aliasDefinition{
	{canonical: "cinder-volume-ceph", variants: [][]string{{"cinder", "volume", "ceph"}}},
	{canonical: "cinder-volume", variants: [][]string{{"cinder", "volume"}}},
	{canonical: "monitor-address", variants: [][]string{{"monitor", "address"}, {"monitor", "addresses"}}},
	{canonical: "relation-changed", variants: [][]string{{"relation", "changed"}}},
	{canonical: "update-status", variants: [][]string{{"update", "status"}}},
	{canonical: "microceph", variants: [][]string{{"microceph"}, {"micro", "ceph"}}},
	{canonical: "in-flight", variants: [][]string{{"in", "flight"}}},
}

type concept struct {
	raw      string
	normal   string
	stem     string
	variants [][]string
	clause   int
	compound bool
}

type indexedField struct {
	name         string
	text         string
	sourceRef    string
	weight       float64
	tokens       []token
	uniqueTokens []token
}

type indexedCandidate struct {
	candidate candidate
	fields    []indexedField
}

type token struct {
	text  string
	stem  string
	start int
	end   int
}

type conceptMatch struct {
	concept      concept
	field        indexedField
	matchType    string
	multiplier   float64
	start        int
	end          int
	contribution float64
}

func conceptsFromText(text string, fuzzy bool) []concept {
	parts := clausePattern.Split(text, -1)
	seen := make(map[string]bool)
	result := make([]concept, 0, len(wordPattern.FindAllString(text, -1)))
	for clause, part := range parts {
		result = append(result, conceptsFromPart(part, fuzzy, clause, seen)...)
	}
	return result
}

func conceptsFromTerms(terms []string, fuzzy bool) []concept {
	seen := make(map[string]bool)
	result := make([]concept, 0, len(terms))
	for clause, term := range terms {
		result = append(result, conceptsFromPart(term, fuzzy, clause, seen)...)
	}
	return result
}

func conceptsFromPart(text string, fuzzy bool, clause int, seen map[string]bool) []concept {
	words := wordPattern.FindAllString(normalizeText(text), -1)
	var result []concept
	for i := 0; i < len(words); {
		if definition, length, ok := aliasAt(words, i); ok {
			if !seen[definition.canonical] {
				seen[definition.canonical] = true
				result = append(result, concept{
					raw:      strings.Join(words[i:i+length], " "),
					normal:   definition.canonical,
					variants: definition.variants,
					clause:   clause,
					compound: length > 1 || strings.Contains(definition.canonical, "-"),
				})
			}
			i += length
			continue
		}
		word := words[i]
		i++
		if stopWords[word] || seen[word] {
			continue
		}
		seen[word] = true
		item := concept{raw: word, normal: word, clause: clause}
		if fuzzy {
			item.stem = stem(word)
		}
		result = append(result, item)
	}
	return result
}

func aliasAt(words []string, offset int) (aliasDefinition, int, bool) {
	for _, definition := range aliasDefinitions {
		for _, variant := range definition.variants {
			if len(variant) == 0 || offset+len(variant) > len(words) {
				continue
			}
			matched := true
			for i := range variant {
				if words[offset+i] != variant[i] {
					matched = false
					break
				}
			}
			if matched {
				return definition, len(variant), true
			}
		}
	}
	return aliasDefinition{}, 0, false
}

func normalizedTerms(terms []string, _ bool) []string {
	result := make([]string, 0, len(terms))
	for _, term := range terms {
		result = append(result, wordPattern.FindAllString(normalizeText(term), -1)...)
	}
	return result
}

func normalizeText(text string) string {
	text = norm.NFKC.String(text)
	var out strings.Builder
	space := false
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			space = false
		} else if !space {
			out.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(out.String())
}

func stem(word string) string {
	stemmed, err := snowball.Stem(word, "english", false)
	if err != nil {
		return word
	}
	return stemmed
}

func tokenizeWithStemCache(text string, fuzzy bool, stemCache map[string]string) []token {
	indices := wordPattern.FindAllStringIndex(text, -1)
	result := make([]token, 0, len(indices))
	for _, index := range indices {
		value := strings.ToLower(text[index[0]:index[1]])
		item := token{text: value, start: index[0], end: index[1]}
		if fuzzy {
			item.stem = stemCache[value]
			if item.stem == "" {
				item.stem = stem(value)
				if stemCache != nil {
					stemCache[value] = item.stem
				}
			}
		}
		result = append(result, item)
	}
	return result
}

func indexCandidates(candidates []candidate, req resolvedRequest) []indexedCandidate {
	result := make([]indexedCandidate, 0, len(candidates))
	stemCache := make(map[string]string)
	for _, item := range candidates {
		result = append(result, indexedCandidate{
			candidate: item,
			fields:    fieldsFor(item, req, stemCache),
		})
	}
	return result
}

func fieldsFor(item candidate, req resolvedRequest, stemCache map[string]string) []indexedField {
	bug := item.document.Bug
	selected := stringSet(req.fields)
	var result []indexedField
	add := func(name, text, source string) {
		if !selected[name] || strings.TrimSpace(text) == "" {
			return
		}
		tokens := tokenizeWithStemCache(text, req.fuzzy, stemCache)
		field := indexedField{
			name: name, text: text, sourceRef: source,
			weight: supportedFields[name], tokens: tokens,
		}
		seenTokens := make(map[string]bool)
		for _, itemToken := range tokens {
			if !seenTokens[itemToken.text] {
				seenTokens[itemToken.text] = true
				field.uniqueTokens = append(field.uniqueTokens, itemToken)
			}
		}
		result = append(result, field)
	}
	add("title", bug.Title, "")
	add("description", bug.Description, "")
	for _, comment := range bug.Comments {
		source := comment.URL
		if source == "" {
			source = "comment:" + comment.Author
		}
		add("comments", comment.Subject+"\n"+comment.Body, source)
	}
	add("tags", strings.Join(bug.Tags, " "), "")
	add("tracker", bug.Forge.String(), "")
	add("id", bug.ID, "")
	add("url", bug.URL, "")
	for _, task := range item.document.Tasks {
		source := taskReference(task)
		add("project", task.Project, source)
		add("status", task.Status, source)
		add("importance", task.Importance, source)
		add("assignee", task.Assignee, source)
		add("tasks", task.Title+" "+task.TargetName, source)
	}
	return result
}

func allConcepts(req resolvedRequest) []concept {
	result := append(append([]concept(nil), req.concepts...), req.required...)
	nextClause := clauseCount(result)
	for _, phrase := range req.phrases {
		normal := normalizeText(phrase)
		result = append(result, concept{
			raw: phrase, normal: normal,
			variants: [][]string{wordPattern.FindAllString(normal, -1)},
			clause:   nextClause, compound: true,
		})
		nextClause++
	}
	return result
}

func conceptDocumentFrequency(candidates []indexedCandidate, req resolvedRequest) map[string]int {
	frequency := make(map[string]int)
	concepts := allConcepts(req)
	for _, item := range candidates {
		for _, itemConcept := range concepts {
			if _, ok := bestDocumentMatch(item.fields, itemConcept, req.fuzzy, 1); ok {
				frequency[itemConcept.normal]++
			}
		}
	}
	return frequency
}

func evaluateText(
	item indexedCandidate,
	req resolvedRequest,
	df map[string]int,
	total int,
) (dto.BugSearchResult, bool) {
	if excludedMatch(item.fields, req.excluded) {
		return dto.BugSearchResult{}, false
	}

	result := resultBase(item.candidate)
	concepts := allConcepts(req)
	requiredStart := len(req.concepts)
	phraseStart := requiredStart + len(req.required)
	matches := make([]conceptMatch, 0, len(concepts))
	weights := make([]float64, len(concepts))
	for i, itemConcept := range concepts {
		weight := inverseDocumentFrequency(total, df[itemConcept.normal])
		if itemConcept.compound {
			weight *= 1.25
		}
		weights[i] = weight
		match, ok := bestDocumentMatch(item.fields, itemConcept, req.fuzzy, weight)
		if !ok {
			if i >= requiredStart {
				return dto.BugSearchResult{}, false
			}
			continue
		}
		if i >= phraseStart {
			match.matchType = "phrase"
			match.multiplier = 1.15
			match.contribution = match.field.weight * match.multiplier * weight
		}
		matches = append(matches, match)
		result.Score += match.contribution
	}
	if len(matches) == 0 {
		return dto.BugSearchResult{}, false
	}

	result.Coverage = coverageFor(concepts, matches, weights)
	result.Classification = classifyCoverage(result.Coverage, matches, len(concepts))
	if result.Classification == "" {
		return dto.BugSearchResult{}, false
	}
	result.Evidence, result.EvidenceTotal, result.EvidenceTruncated =
		groupEvidence(matches, req.evidenceLimit)
	return result, true
}

func evaluateRegex(item indexedCandidate, req resolvedRequest) (dto.BugSearchResult, bool) {
	result := resultBase(item.candidate)
	bestByField := make(map[string]conceptMatch)
	regexConcept := concept{raw: req.Query, normal: req.Query, clause: 0}
	for _, field := range item.fields {
		location := req.regex.FindStringIndex(field.text)
		if location == nil {
			continue
		}
		match := conceptMatch{
			concept: regexConcept, field: field, matchType: "regex",
			multiplier: 1, start: location[0], end: location[1],
			contribution: field.weight,
		}
		if previous, ok := bestByField[field.name]; !ok || match.contribution > previous.contribution {
			bestByField[field.name] = match
		}
	}
	if len(bestByField) == 0 {
		return dto.BugSearchResult{}, false
	}
	var matches []conceptMatch
	for _, match := range bestByField {
		matches = append(matches, match)
		result.Score += match.contribution
	}
	result.Classification = dto.BugSearchRelated
	for _, match := range matches {
		if highSignal(match.field.name) {
			result.Classification = dto.BugSearchDirect
			break
		}
	}
	result.Coverage = BugSearchCoverageForRegex(matches)
	result.Evidence, result.EvidenceTotal, result.EvidenceTruncated =
		groupEvidence(matches, req.evidenceLimit)
	return result, true
}

func searchDirectReference(
	candidates []candidate,
	req resolvedRequest,
	reference string,
) *dto.BugSearchResponse {
	for _, item := range candidates {
		candidateReference := strings.ToLower(item.document.Bug.Forge.String()) + ":" + item.document.Bug.ID
		if !strings.EqualFold(reference, candidateReference) {
			continue
		}
		result := resultBase(item)
		result.Classification = dto.BugSearchDirect
		result.Score = 100
		result.Coverage = dto.BugSearchCoverage{
			MatchedConcepts: []string{reference}, MatchedWeight: 1,
			TotalWeight: 1, Ratio: 1, MatchedClauses: 1, TotalClauses: 1,
		}
		result.Evidence = []dto.BugMatchEvidence{{
			Field: "id", Excerpt: item.document.Bug.ID,
			Matches: []dto.BugEvidenceMatch{{
				QueryConcept: req.Query, MatchedText: item.document.Bug.ID, MatchType: "exact",
			}},
		}}
		result.EvidenceTotal = 1
		return finishResponse([]dto.BugSearchResult{result}, nil, req, len(candidates))
	}
	return emptyResponse(req, len(candidates))
}

func expandTaskScopedResults(
	result dto.BugSearchResult,
	item candidate,
	merge bool,
) []dto.BugSearchResult {
	if merge || len(item.matchedTasks) == 0 {
		return []dto.BugSearchResult{result}
	}
	results := make([]dto.BugSearchResult, 0, len(item.matchedTasks))
	for _, task := range item.matchedTasks {
		scoped := result
		sourceRef := taskReference(task)
		scoped.Tasks = []forge.BugTask{task}
		scoped.Projects = []string{task.Project}
		scoped.Status = []string{task.Status}
		scoped.Importance = []string{task.Importance}
		scoped.MatchedTasks = []string{sourceRef}
		scoped.Evidence = filterTaskEvidence(result.Evidence, sourceRef)
		scoped.EvidenceTotal = len(scoped.Evidence)
		scoped.EvidenceTruncated = false
		results = append(results, scoped)
	}
	return results
}

func filterTaskEvidence(evidence []dto.BugMatchEvidence, taskRef string) []dto.BugMatchEvidence {
	result := make([]dto.BugMatchEvidence, 0, len(evidence))
	for _, item := range evidence {
		if item.SourceReference == "" || item.SourceReference == taskRef {
			result = append(result, item)
		}
	}
	return result
}

func taskReference(task forge.BugTask) string {
	if task.TargetName == "" {
		return task.Project + ":" + task.BugID
	}
	return task.Project + ":" + task.TargetName
}

func bestDocumentMatch(
	fields []indexedField,
	itemConcept concept,
	fuzzy bool,
	conceptWeight float64,
) (conceptMatch, bool) {
	var best conceptMatch
	for _, field := range fields {
		match, ok := bestConceptMatch(field, itemConcept, fuzzy)
		if !ok {
			continue
		}
		match.contribution = field.weight * match.multiplier * conceptWeight
		if match.contribution > best.contribution ||
			(match.contribution == best.contribution && evidenceMatchLess(match, best)) {
			best = match
		}
	}
	return best, best.contribution > 0
}

func bestConceptMatch(field indexedField, itemConcept concept, fuzzy bool) (conceptMatch, bool) {
	best := conceptMatch{}
	for _, variant := range itemConcept.variants {
		if start, end, ok := phraseLocation(field.tokens, variant); ok {
			multiplier := 1.0
			matchType := "alias"
			if strings.Join(variant, "-") == itemConcept.normal ||
				strings.Join(variant, " ") == itemConcept.normal {
				matchType = "exact"
			}
			if multiplier > best.multiplier {
				best = conceptMatch{
					concept: itemConcept, field: field, matchType: matchType,
					multiplier: multiplier, start: start, end: end,
				}
			}
		}
	}
	if itemConcept.compound {
		return best, best.multiplier > 0
	}
	var stemMatch token
	for _, fieldToken := range field.uniqueTokens {
		if fieldToken.text == itemConcept.normal {
			return conceptMatch{
				concept: itemConcept, field: field, matchType: "exact",
				multiplier: 1, start: fieldToken.start, end: fieldToken.end,
			}, true
		}
		if fuzzy && itemConcept.stem != "" &&
			fieldToken.stem == itemConcept.stem && stemMatch.text == "" {
			stemMatch = fieldToken
		}
	}
	if stemMatch.text != "" {
		return conceptMatch{
			concept: itemConcept, field: field, matchType: "stem",
			multiplier: 0.9, start: stemMatch.start, end: stemMatch.end,
		}, true
	}
	for _, fieldToken := range field.uniqueTokens {
		matchType, multiplier := "", 0.0
		if fuzzy && withinFuzzyDistance(fieldToken.text, itemConcept.normal) {
			matchType, multiplier = "fuzzy", 0.6
		}
		if multiplier > best.multiplier {
			best = conceptMatch{
				concept: itemConcept, field: field, matchType: matchType,
				multiplier: multiplier, start: fieldToken.start, end: fieldToken.end,
			}
		}
	}
	return best, best.multiplier > 0
}

func phraseLocation(tokens []token, phrase []string) (int, int, bool) {
	if len(phrase) == 0 {
		return 0, 0, false
	}
	for i := 0; i+len(phrase) <= len(tokens); i++ {
		matched := true
		for j := range phrase {
			if tokens[i+j].text != phrase[j] {
				matched = false
				break
			}
		}
		if matched {
			return tokens[i].start, tokens[i+len(phrase)-1].end, true
		}
	}
	return 0, 0, false
}

func excludedMatch(fields []indexedField, excluded []string) bool {
	if len(excluded) == 0 {
		return false
	}
	set := stringSet(excluded)
	for _, field := range fields {
		for _, fieldToken := range field.tokens {
			if set[fieldToken.text] {
				return true
			}
		}
	}
	return false
}

func inverseDocumentFrequency(total, frequency int) float64 {
	if frequency < 1 {
		frequency = 1
	}
	value := math.Log((float64(total)+1)/(float64(frequency)+1)) + 1
	return math.Min(4, math.Max(1, value))
}

func coverageFor(
	concepts []concept,
	matches []conceptMatch,
	weights []float64,
) dto.BugSearchCoverage {
	matched := make(map[string]bool)
	matchedClauses := make(map[int]bool)
	for _, match := range matches {
		matched[match.concept.normal] = true
		matchedClauses[match.concept.clause] = true
	}
	totalClauses := make(map[int]bool)
	coverage := dto.BugSearchCoverage{}
	for i, itemConcept := range concepts {
		totalClauses[itemConcept.clause] = true
		coverage.TotalWeight += weights[i]
		if matched[itemConcept.normal] {
			coverage.MatchedConcepts = append(coverage.MatchedConcepts, itemConcept.normal)
			coverage.MatchedWeight += weights[i]
		} else {
			coverage.UnmatchedConcepts = append(coverage.UnmatchedConcepts, itemConcept.normal)
		}
	}
	if coverage.TotalWeight > 0 {
		coverage.Ratio = coverage.MatchedWeight / coverage.TotalWeight
	}
	coverage.MatchedClauses = len(matchedClauses)
	coverage.TotalClauses = len(totalClauses)
	return coverage
}

func classifyCoverage(
	coverage dto.BugSearchCoverage,
	matches []conceptMatch,
	totalConcepts int,
) dto.BugSearchClassification {
	highSignalMatch := false
	distinctiveMatch := false
	for _, match := range matches {
		if highSignal(match.field.name) {
			highSignalMatch = true
		}
		if match.concept.compound || match.contribution >= match.field.weight*1.2 {
			distinctiveMatch = true
		}
	}
	if coverage.Ratio == 1 {
		distinctiveMatch = true
	}
	if totalConcepts == 1 {
		if highSignalMatch && matches[0].matchType != "fuzzy" {
			return dto.BugSearchDirect
		}
		if highSignalMatch {
			return dto.BugSearchPartial
		}
		return dto.BugSearchRelated
	}
	clauseRatio := 0.0
	if coverage.TotalClauses > 0 {
		clauseRatio = float64(coverage.MatchedClauses) / float64(coverage.TotalClauses)
	}
	switch {
	case coverage.Ratio >= 0.68 && clauseRatio >= 0.6 &&
		len(coverage.MatchedConcepts) >= 2 && highSignalMatch && distinctiveMatch:
		return dto.BugSearchDirect
	case coverage.Ratio >= 0.35 && clauseRatio >= 0.4 &&
		len(coverage.MatchedConcepts) >= 2:
		return dto.BugSearchPartial
	case distinctiveMatch || len(coverage.MatchedConcepts) >= 2:
		return dto.BugSearchRelated
	default:
		return ""
	}
}

func clauseCount(concepts []concept) int {
	maxClause := -1
	for _, itemConcept := range concepts {
		if itemConcept.clause > maxClause {
			maxClause = itemConcept.clause
		}
	}
	return maxClause + 1
}

func groupEvidence(
	matches []conceptMatch,
	limit int,
) ([]dto.BugMatchEvidence, int, bool) {
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].field.name != matches[j].field.name {
			return supportedFields[matches[i].field.name] > supportedFields[matches[j].field.name]
		}
		if matches[i].field.sourceRef != matches[j].field.sourceRef {
			return matches[i].field.sourceRef < matches[j].field.sourceRef
		}
		return matches[i].start < matches[j].start
	})

	type group struct {
		field   indexedField
		start   int
		end     int
		matches []conceptMatch
		score   float64
	}
	var groups []group
	for _, match := range matches {
		merged := false
		for i := range groups {
			current := &groups[i]
			mergedStart := minInt(current.start, match.start)
			mergedEnd := maxInt(current.end, match.end)
			if current.field.name == match.field.name &&
				current.field.sourceRef == match.field.sourceRef &&
				match.start <= current.end+96 && match.end >= current.start-96 &&
				mergedEnd-mergedStart <= 160 {
				current.start = mergedStart
				current.end = mergedEnd
				current.matches = append(current.matches, match)
				current.score += match.contribution
				merged = true
				break
			}
		}
		if !merged {
			groups = append(groups, group{
				field: match.field, start: match.start, end: match.end,
				matches: []conceptMatch{match}, score: match.contribution,
			})
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].score != groups[j].score {
			return groups[i].score > groups[j].score
		}
		if groups[i].field.name != groups[j].field.name {
			return supportedFields[groups[i].field.name] > supportedFields[groups[j].field.name]
		}
		return groups[i].field.sourceRef < groups[j].field.sourceRef
	})
	total := len(groups)
	if len(groups) > limit {
		groups = groups[:limit]
	}
	evidence := make([]dto.BugMatchEvidence, 0, len(groups))
	for _, current := range groups {
		evidence = append(evidence, evidenceForGroup(
			current.field, current.start, current.end, current.matches,
		))
	}
	return evidence, total, total > len(evidence)
}

func evidenceForGroup(
	field indexedField,
	start, end int,
	matches []conceptMatch,
) dto.BugMatchEvidence {
	const contextRunes = 64
	const maxExcerptRunes = 240
	runes := []rune(field.text)
	startRune := utf8.RuneCountInString(field.text[:start])
	endRune := startRune + utf8.RuneCountInString(field.text[start:end])
	excerptStart := maxInt(0, startRune-contextRunes)
	excerptEnd := minInt(len(runes), endRune+contextRunes)
	if excerptEnd-excerptStart > maxExcerptRunes {
		excerptEnd = minInt(len(runes), excerptStart+maxExcerptRunes)
	}
	item := dto.BugMatchEvidence{
		Field: field.name, SourceReference: field.sourceRef,
		Excerpt:         strings.TrimSpace(string(runes[excerptStart:excerptEnd])),
		TruncatedBefore: excerptStart > 0,
		TruncatedAfter:  excerptEnd < len(runes),
	}
	seen := make(map[string]bool)
	for _, match := range matches {
		key := match.concept.normal + "\x00" + match.matchType
		if seen[key] {
			continue
		}
		seen[key] = true
		matchedText := field.text[match.start:match.end]
		if utf8.RuneCountInString(matchedText) > 80 {
			matchedText = string([]rune(matchedText)[:80])
			item.MatchTruncated = true
		}
		item.Matches = append(item.Matches, dto.BugEvidenceMatch{
			QueryConcept: match.concept.normal,
			MatchedText:  matchedText,
			MatchType:    match.matchType,
		})
	}
	return item
}

func evidenceMatchLess(left, right conceptMatch) bool {
	if supportedFields[left.field.name] != supportedFields[right.field.name] {
		return supportedFields[left.field.name] > supportedFields[right.field.name]
	}
	if left.field.sourceRef != right.field.sourceRef {
		return left.field.sourceRef < right.field.sourceRef
	}
	return left.start < right.start
}

func BugSearchCoverageForRegex(matches []conceptMatch) dto.BugSearchCoverage {
	coverage := dto.BugSearchCoverage{
		MatchedConcepts: []string{"regex"}, MatchedWeight: 1,
		TotalWeight: 1, Ratio: 1, MatchedClauses: 1, TotalClauses: 1,
	}
	if len(matches) == 0 {
		coverage.MatchedConcepts = nil
		coverage.MatchedWeight = 0
		coverage.Ratio = 0
		coverage.MatchedClauses = 0
	}
	return coverage
}

func withinFuzzyDistance(left, right string) bool {
	length := utf8.RuneCountInString(right)
	var maxDistance int
	if length >= 8 {
		maxDistance = 2
	} else if length >= 4 {
		maxDistance = 1
	} else {
		return false
	}
	return levenshtein(left, right, maxDistance) <= maxDistance
}

func levenshtein(left, right string, maxDistance int) int {
	a, b := []rune(left), []rune(right)
	if absInt(len(a)-len(b)) > maxDistance {
		return maxDistance + 1
	}
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, ar := range a {
		current := make([]int, len(b)+1)
		current[0] = i + 1
		rowMin := current[0]
		for j, br := range b {
			cost := 0
			if ar != br {
				cost = 1
			}
			current[j+1] = minInt(current[j]+1, previous[j+1]+1, previous[j]+cost)
			rowMin = minInt(rowMin, current[j+1])
		}
		if rowMin > maxDistance {
			return maxDistance + 1
		}
		previous = current
	}
	return previous[len(b)]
}

func highSignal(field string) bool {
	return field == "title" || field == "description" || field == "comments"
}

func minInt(values ...int) int {
	result := math.MaxInt
	for _, value := range values {
		if value < result {
			result = value
		}
	}
	return result
}

func maxInt(values ...int) int {
	result := math.MinInt
	for _, value := range values {
		if value > result {
			result = value
		}
	}
	return result
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
