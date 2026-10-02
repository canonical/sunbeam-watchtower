// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package excusescache

import (
	"strings"
	"unicode"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
	"golang.org/x/net/html"
)

// stripHTML removes all HTML tags from s using the golang.org/x/net/html
// tokenizer, keeping only text content. HTML entities are decoded and control
// characters (except tab, newline, carriage return) are stripped.
func stripHTML(s string) string {
	if !strings.ContainsAny(s, "<&") {
		return sanitizeText(s)
	}
	tokenizer := html.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.TextToken {
			b.Write(tokenizer.Text())
		}
	}
	return sanitizeText(b.String())
}

// sanitizeText strips control characters and Unicode directional overrides
// from text to prevent terminal escape injection.
func sanitizeText(s string) string {
	return strings.Map(func(r rune) rune {
		if isUnsafeRune(r) {
			return -1
		}
		return r
	}, s)
}

// isUnsafeRune returns true for control characters (except whitespace) and
// Unicode directional overrides that could be used for terminal injection.
func isUnsafeRune(r rune) bool {
	// Allow tab, newline, carriage return.
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	if unicode.IsControl(r) {
		return true
	}
	// Unicode directional overrides.
	switch {
	case r >= 0x202A && r <= 0x202E: // LRE, RLE, PDF, LRO, RLO
		return true
	case r >= 0x2066 && r <= 0x2069: // LRI, RLI, FSI, PDI
		return true
	case r == 0x200E || r == 0x200F: // LRM, RLM
		return true
	case r == 0x2028 || r == 0x2029: // LINE SEPARATOR, PARAGRAPH SEPARATOR
		return true
	}
	return false
}

// parseAutopkgtestLine parses a britney autopkgtest HTML line into structured
// per-architecture entries. Returns nil if the line is not an autopkgtest line.
//
// Expected format:
//
//	autopkgtest for pkg/ver: <a href="ARCH_URL">arch</a>: <a href="LOG_URL"><span ...>STATUS</span></a>, ...
func parseAutopkgtestLine(line string) []dto.ExcuseAutopkgtest {
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "autopkgtest") {
		return nil
	}

	// Extract the triggering package from the preamble text.
	// Format: "autopkgtest for pkg/version: ..."
	triggeringPackage := sanitizeText(extractTriggeringPackage(line))

	// Pre-sanitize the raw line for storage in Message (backward compat).
	// Strip HTML tags but keep text content; sanitize control chars.
	sanitizedMessage := stripHTML(line)

	tokenizer := html.NewTokenizer(strings.NewReader(line))

	var results []dto.ExcuseAutopkgtest
	var current autopkgtestParseState

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.StartTagToken:
			tn, hasAttr := tokenizer.TagName()
			tagName := string(tn)

			switch tagName {
			case "a":
				href := extractAttr(tokenizer, hasAttr, "href")
				current.handleAnchorStart(href)
			case "span":
				current.inSpan = true
			}

		case html.TextToken:
			text := sanitizeText(strings.TrimSpace(string(tokenizer.Text())))
			if text == "" {
				continue
			}
			if current.inSpan {
				current.statusText = text
			} else if current.inArchAnchor {
				current.archText = text
			}

		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			tagName := string(tn)

			switch tagName {
			case "a":
				if current.inStatusAnchor && current.statusText != "" {
					results = append(results, dto.ExcuseAutopkgtest{
						Package:      triggeringPackage,
						Architecture: current.archText,
						Status:       normalizeAutopkgtestStatus(current.statusText),
						URL:          current.statusHref,
						Message:      sanitizedMessage,
					})
					current.reset()
				}
				current.inArchAnchor = false
				current.inStatusAnchor = false
			case "span":
				current.inSpan = false
			}
		}
	}

	if len(results) == 0 {
		// Fallback: line mentions autopkgtest but HTML parsing yielded nothing.
		return []dto.ExcuseAutopkgtest{{
			Package: triggeringPackage,
			Status:  "unknown",
			Message: sanitizedMessage,
		}}
	}

	return results
}

type autopkgtestParseState struct {
	inArchAnchor   bool
	inStatusAnchor bool
	inSpan         bool
	archText       string
	statusText     string
	statusHref     string
}

func (s *autopkgtestParseState) handleAnchorStart(href string) {
	if s.archText == "" {
		// First anchor in a pair: the architecture link.
		s.inArchAnchor = true
		s.inStatusAnchor = false
	} else {
		// Second anchor: the status/log link. Sanitize the URL to prevent
		// control characters from breaking OSC 8 terminal escape sequences.
		s.inArchAnchor = false
		s.inStatusAnchor = true
		s.statusHref = sanitizeText(href)
	}
}

func (s *autopkgtestParseState) reset() {
	s.inArchAnchor = false
	s.inStatusAnchor = false
	s.inSpan = false
	s.archText = ""
	s.statusText = ""
	s.statusHref = ""
}

// extractTriggeringPackage extracts the "pkg/version" from the preamble
// text "autopkgtest for pkg/version: <a ...". The separator is ": <"
// (colon-space before the first HTML tag), which avoids splitting on epoch
// colons in Debian version strings like "1:29.1.0-0ubuntu1".
func extractTriggeringPackage(line string) string {
	lower := strings.ToLower(line)
	idx := strings.Index(lower, "autopkgtest for ")
	if idx < 0 {
		return ""
	}
	rest := line[idx+len("autopkgtest for "):]
	// Look for ": <" which separates the package/version from the HTML body.
	if sep := strings.Index(rest, ": <"); sep > 0 {
		return strings.TrimSpace(rest[:sep])
	}
	// Fallback: no HTML, take up to the first colon-space.
	if sep := strings.Index(rest, ": "); sep > 0 {
		return strings.TrimSpace(rest[:sep])
	}
	return strings.TrimSpace(rest)
}

// extractAttr returns the value of the named attribute from the current token.
func extractAttr(tokenizer *html.Tokenizer, hasAttr bool, name string) string {
	for hasAttr {
		var key, val []byte
		key, val, hasAttr = tokenizer.TagAttr()
		if string(key) == name {
			return string(val)
		}
	}
	return ""
}

// normalizeAutopkgtestStatus maps raw status text from britney HTML spans
// to canonical lowercase status strings.
func normalizeAutopkgtestStatus(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case lower == "pass":
		return "pass"
	case strings.HasPrefix(lower, "test in progress"):
		return "in-progress"
	case lower == "no test results":
		return "no-results"
	case lower == "not a regression":
		return "not-regression"
	case lower == "always failed":
		return "always-failed"
	case lower == "regression":
		return "regression"
	default:
		return lower
	}
}
