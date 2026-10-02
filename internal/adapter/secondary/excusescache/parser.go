// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package excusescache

import (
	"fmt"
	"strconv"
	"strings"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"gopkg.in/yaml.v3"
)

type excusesDocument struct {
	Sources []map[string]any `yaml:"sources"`
}

func parseExcusesYAML(data []byte, source dto.ExcusesSource) ([]dto.PackageExcuse, error) {
	var doc excusesDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshalling excuses yaml: %w", err)
	}

	results := make([]dto.PackageExcuse, 0, len(doc.Sources))
	for _, entry := range doc.Sources {
		var excuse dto.PackageExcuse
		var err error
		provider := source.Provider
		if provider == "" {
			provider = source.Tracker
		}
		switch provider {
		case dto.ExcusesTrackerUbuntu:
			excuse, err = parseUbuntuExcuse(entry)
		case dto.ExcusesTrackerDebian:
			excuse, err = parseDebianExcuse(entry)
		default:
			excuse, err = parseGenericExcuse(entry)
		}
		if err != nil {
			return nil, err
		}
		excuse.Tracker = source.Tracker
		results = append(results, excuse)
	}

	applyReverseDependencyCounts(results)
	return results, nil
}

func parseUbuntuExcusesByTeamYAML(data []byte) (map[string]string, map[string]string, error) {
	exactTeams := map[string]string{}
	packageTeams := map[string]string{}

	var currentTeam string
	var currentPackage string
	var currentVersion string
	finalize := func() {
		if currentTeam == "" || currentPackage == "" {
			return
		}
		packageTeams[currentPackage] = currentTeam
		if currentVersion != "" {
			exactTeams[recordKey(currentPackage, currentVersion)] = currentTeam
		}
	}

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))
		switch {
		case indent == 0 && strings.HasSuffix(trimmed, ":"):
			finalize()
			currentTeam = strings.TrimSuffix(trimmed, ":")
			currentPackage = ""
			currentVersion = ""
		case indent <= 2 && strings.HasPrefix(trimmed, "- "):
			finalize()
			currentPackage = ""
			currentVersion = ""
		default:
			if pkg, ok := parseSimpleYAMLValue(trimmed, "package_in_proposed"); ok {
				currentPackage = pkg
				continue
			}
			if version, ok := parseSimpleYAMLValue(trimmed, "new-version"); ok && currentVersion == "" {
				currentVersion = version
			}
		}
	}

	finalize()

	return exactTeams, packageTeams, nil
}

func parseUbuntuExcuse(entry map[string]any) (dto.PackageExcuse, error) {
	excuse, err := parseGenericExcuse(entry)
	if err != nil {
		return dto.PackageExcuse{}, err
	}

	policyInfo := nestedMap(entry, "policy_info")
	excuse.AgeDays = nestedInt(policyInfo, "age", "current-age")
	if excuse.Bug == "" {
		if bug := firstBugKey(nestedMap(policyInfo, "update-excuse")); bug != "" {
			excuse.Bug = "LP: #" + bug
		} else if bug := firstBugKey(nestedMap(policyInfo, "block-bugs")); bug != "" {
			excuse.Bug = "LP: #" + bug
		}
	}

	for _, arch := range nestedStringSlice(entry, "missing-builds", "on-architectures") {
		excuse.FTBFS = true
		excuse.BuildFailures = append(excuse.BuildFailures, dto.ExcuseBuildFailure{
			Architecture: arch,
			Kind:         "missing-build",
			Message:      "missing build on " + arch,
		})
	}

	return finalizeExcuse(excuse), nil
}

func parseDebianExcuse(entry map[string]any) (dto.PackageExcuse, error) {
	excuse, err := parseGenericExcuse(entry)
	if err != nil {
		return dto.PackageExcuse{}, err
	}

	for _, hint := range sliceOfMaps(entry["hints"]) {
		hintType := stringValue(hint["hint-type"])
		hintFrom := stringValue(hint["hint-from"])
		if hintType == "" && hintFrom == "" {
			continue
		}
		msg := strings.TrimSpace(strings.Join([]string{hintType, hintFrom}, " from "))
		excuse.Reasons = append(excuse.Reasons, dto.ExcuseReason{
			Code:     normalizeReasonCode(hintType),
			Message:  msg,
			Blocking: false,
		})
	}

	return finalizeExcuse(excuse), nil
}

func parseGenericExcuse(entry map[string]any) (dto.PackageExcuse, error) {
	excuse := dto.PackageExcuse{}
	excuse.Package = stringValue(entry["source"])
	excuse.ItemName = stringValue(entry["item-name"])
	excuse.Version = stringValue(entry["new-version"])
	excuse.OldVersion = stringValue(entry["old-version"])
	excuse.Component = stringValue(entry["component"])
	excuse.Candidate = boolValue(entry["is-candidate"])
	excuse.Verdict = stringValue(entry["migration-policy-verdict"])
	excuse.Team = stringValue(entry["team"])
	excuse.Maintainer = stringValue(entry["maintainer"])
	excuse.Messages = stringSlice(entry["excuses"])
	excuse.BlockedBy = append(excuse.BlockedBy, nestedStringSlice(entry, "dependencies", "blocked-by")...)

	for _, pkg := range nestedStringSlice(entry, "dependencies", "migrate-after") {
		excuse.Dependencies = append(excuse.Dependencies, dto.ExcuseDependency{
			Kind:    "migrate-after",
			Package: pkg,
		})
	}
	for _, pkg := range excuse.BlockedBy {
		excuse.Dependencies = append(excuse.Dependencies, dto.ExcuseDependency{
			Kind:    "blocked-by",
			Package: pkg,
		})
	}

	for _, reason := range stringSlice(entry["reason"]) {
		excuse.Reasons = append(excuse.Reasons, dto.ExcuseReason{
			Code:     normalizeReasonCode(reason),
			Message:  reason,
			Blocking: true,
		})
		if reason == "no-binaries" {
			excuse.FTBFS = true
		}
	}

	var cleanMessages []string
	for _, line := range excuse.Messages {
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "autopkgtest"):
			excuse.Autopkgtests = append(excuse.Autopkgtests, parseAutopkgtestLine(line)...)
		case strings.Contains(lower, "build"):
			cleaned := stripHTML(line)
			excuse.BuildFailures = append(excuse.BuildFailures, dto.ExcuseBuildFailure{
				Message: cleaned,
				Kind:    normalizeReasonCode(cleaned),
			})
		default:
			cleanMessages = append(cleanMessages, stripHTML(line))
		}
	}
	excuse.Messages = cleanMessages

	return excuse, nil
}

func finalizeExcuse(excuse dto.PackageExcuse) dto.PackageExcuse {
	excuse.BlockedBy = dedupeStrings(excuse.BlockedBy)
	excuse.ReverseDependencies = dedupeStrings(excuse.ReverseDependencies)
	excuse.PrimaryReason = derivePrimaryReason(excuse)
	if excuse.PrimaryReason == "ftbfs" {
		excuse.FTBFS = true
	}
	return excuse
}

func applyExcuseTeams(excuses []dto.PackageExcuse, exactTeams, packageTeams map[string]string) {
	for i := range excuses {
		if team := exactTeams[recordKey(excuses[i].Package, excuses[i].Version)]; team != "" {
			excuses[i].Team = team
			continue
		}
		if team := packageTeams[excuses[i].Package]; team != "" {
			excuses[i].Team = team
		}
	}
}

func applyReverseDependencyCounts(excuses []dto.PackageExcuse) {
	blockCounts := map[string]int{}
	reverse := map[string][]string{}
	for _, excuse := range excuses {
		for _, dep := range excuse.Dependencies {
			if dep.Package == "" {
				continue
			}
			blockCounts[dep.Package]++
			reverse[dep.Package] = append(reverse[dep.Package], excuse.Package)
		}
	}

	for i := range excuses {
		excuses[i].BlocksCount = blockCounts[excuses[i].Package]
		excuses[i].ReverseDependencies = dedupeStrings(reverse[excuses[i].Package])
	}
}

func derivePrimaryReason(excuse dto.PackageExcuse) string {
	if excuse.FTBFS || len(excuse.BuildFailures) > 0 {
		return "ftbfs"
	}
	if len(excuse.Autopkgtests) > 0 {
		return "autopkgtest"
	}
	if len(excuse.Dependencies) > 0 {
		return "dependency"
	}
	if len(excuse.Reasons) > 0 {
		return excuse.Reasons[0].Code
	}
	if len(excuse.Messages) > 0 {
		return normalizeReasonCode(excuse.Messages[0])
	}
	return "other"
}

func normalizeReasonCode(reason string) string {
	lower := strings.ToLower(strings.TrimSpace(reason))
	switch {
	case lower == "" || lower == "-":
		return "other"
	case strings.Contains(lower, "autopkgtest"):
		return "autopkgtest"
	case strings.Contains(lower, "ftbfs"), strings.Contains(lower, "no-binaries"), strings.Contains(lower, "missing build"):
		return "ftbfs"
	case strings.Contains(lower, "depend"), strings.Contains(lower, "blocked"):
		return "dependency"
	case strings.Contains(lower, "remove"):
		return "removal"
	case strings.Contains(lower, "freeze"):
		return "freeze"
	default:
		return strings.ReplaceAll(lower, " ", "-")
	}
}

func parseSimpleYAMLValue(line, key string) (string, bool) {
	prefix := key + ":"
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	value = strings.Trim(value, `"'`)
	return value, true
}

func firstBugKey(m map[string]any) string {
	for key := range m {
		if key != "" && key != "verdict" {
			return key
		}
	}
	return ""
}

func nestedMap(m map[string]any, keys ...string) map[string]any {
	cur := m
	for _, key := range keys {
		next, ok := cur[key]
		if !ok {
			return nil
		}
		cur = mapValue(next)
		if cur == nil {
			return nil
		}
	}
	return cur
}

func nestedInt(m map[string]any, keys ...string) int {
	cur := nestedMap(m, keys[:len(keys)-1]...)
	if cur == nil {
		return 0
	}
	return intValue(cur[keys[len(keys)-1]])
}

func nestedStringSlice(m map[string]any, keys ...string) []string {
	cur := nestedMap(m, keys[:len(keys)-1]...)
	if cur == nil {
		return nil
	}
	return stringSlice(cur[keys[len(keys)-1]])
}

func mapValue(v any) map[string]any {
	if v == nil {
		return nil
	}
	switch typed := v.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			out[stringValue(k)] = val
		}
		return out
	default:
		return nil
	}
}

func stringValue(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func boolValue(v any) bool {
	switch typed := v.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true")
	default:
		return false
	}
}

func intValue(v any) int {
	switch typed := v.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		n, _ := strconv.Atoi(typed)
		return n
	default:
		return 0
	}
}

func stringSlice(v any) []string {
	switch typed := v.(type) {
	case []string:
		return dedupeStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := stringValue(item); s != "" {
				out = append(out, s)
			}
		}
		return dedupeStrings(out)
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	default:
		return nil
	}
}

func sliceOfMaps(v any) []map[string]any {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m := mapValue(item); m != nil {
			out = append(out, m)
		}
	}
	return out
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
