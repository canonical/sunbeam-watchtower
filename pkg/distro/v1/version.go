// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"regexp"
	"strings"

	"pault.ag/go/debian/version"
)

var upstreamReleaseVersion = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)*)(?:~?rc([0-9]+))?$`)

// CompareVersions compares two Debian version strings using dpkg semantics.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func CompareVersions(a, b string) int {
	va, err := version.Parse(a)
	if err != nil {
		return strings.Compare(a, b)
	}
	vb, err := version.Parse(b)
	if err != nil {
		return strings.Compare(a, b)
	}
	return version.Compare(va, vb)
}

// StripDebianRevision removes the Debian revision from a version string.
// For example, "3:32.0.0-0ubuntu1" becomes "3:32.0.0".
func StripDebianRevision(v string) string {
	parsed, err := version.Parse(v)
	if err != nil {
		return v
	}
	parsed.Revision = ""
	return parsed.String()
}

// CompareUpstreamVersions compares a packaged version with an upstream tag.
// Numeric PEP 440 release segments ignore trailing zeros, and an OpenStack
// rc suffix has the same meaning as Debian's ~rc spelling. Other version
// forms retain Debian ordering.
func CompareUpstreamVersions(packaged, upstream string) int {
	packagedBase := StripDebianRevision(packaged)
	if a, ok := normalizeUpstreamVersion(packagedBase); ok {
		if b, ok := normalizeUpstreamVersion(upstream); ok {
			return versionSign(CompareVersions(a, b))
		}
	}
	return versionSign(CompareVersions(packaged, upstream))
}

func versionSign(comparison int) int {
	switch {
	case comparison < 0:
		return -1
	case comparison > 0:
		return 1
	default:
		return 0
	}
}

func normalizeUpstreamVersion(v string) (string, bool) {
	match := upstreamReleaseVersion.FindStringSubmatch(v)
	if match == nil {
		return "", false
	}
	segments := strings.Split(match[1], ".")
	for i, segment := range segments {
		segments[i] = normalizeDigits(segment)
	}
	for len(segments) > 1 && segments[len(segments)-1] == "0" {
		segments = segments[:len(segments)-1]
	}
	normalized := strings.Join(segments, ".")
	if match[2] != "" {
		normalized += "~rc" + normalizeDigits(match[2])
	}
	return normalized, true
}

func normalizeDigits(v string) string {
	v = strings.TrimLeft(v, "0")
	if v == "" {
		return "0"
	}
	return v
}

// PickHighest returns the source package with the highest version from a slice.
// Returns nil if the slice is empty.
func PickHighest(versions []SourcePackage) *SourcePackage {
	if len(versions) == 0 {
		return nil
	}
	best := &versions[0]
	for i := 1; i < len(versions); i++ {
		if CompareVersions(versions[i].Version, best.Version) > 0 {
			best = &versions[i]
		}
	}
	return best
}
