// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package distrocache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	distro "github.com/canonical/sunbeam-watchtower/pkg/distro/v1"
)

// ParseSources reads an RFC822-format Sources file and yields SourcePackage entries.
// The suite and component are set from the caller (known from the download path).
func ParseSources(r io.Reader, suite, component string) ([]distro.SourcePackage, error) {
	return distro.ParseSources(r, suite, component)
}

// ParseSourcesDetailed reads an RFC822-format Sources file and yields SourcePackageDetail entries
// including parsed Build-Depends and Build-Depends-Indep.
func ParseSourcesDetailed(r io.Reader, suite, component string) ([]distro.SourcePackageDetail, error) {
	return distro.ParseSourcesDetailed(r, suite, component)
}

// ParseSourcesWithFiles reads an RFC822-format Sources file and yields SourcePackageFiles entries
// including Directory and Files fields needed for .dsc URL construction.
func ParseSourcesWithFiles(r io.Reader, suite, component string) ([]distro.SourcePackageFiles, error) {
	return distro.ParseSourcesWithFiles(r, suite, component)
}

// ParseSourcesFileWithFiles opens and parses a compressed Sources file from disk,
// returning entries with Directory and Files information.
func ParseSourcesFileWithFiles(path, format, suite, component string) ([]distro.SourcePackageFiles, error) {
	return distro.ParseSourcesFileWithFiles(path, format, suite, component)
}

// sourcesURL builds the URL for a Sources index file.
func sourcesURL(mirror, suite, component, format string) string {
	return fmt.Sprintf("%s/dists/%s/%s/source/Sources.%s",
		strings.TrimRight(mirror, "/"), suite, component, format)
}

// downloadSourcesFile downloads a Sources index file to the given path.
// It tries .xz first, then falls back to .gz.
// Returns the format that succeeded ("xz" or "gz").
func downloadSourcesFile(ctx context.Context, client *http.Client, mirror, suite, component, destPath string) (string, error) {
	for _, format := range []string{"xz", "gz"} {
		url := sourcesURL(mirror, suite, component, format)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", fmt.Errorf("creating request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		out, err := os.Create(destPath)
		if err != nil {
			resp.Body.Close()
			return "", fmt.Errorf("creating file %s: %w", destPath, err)
		}

		_, copyErr := io.Copy(out, resp.Body)
		resp.Body.Close()
		out.Close()

		if copyErr != nil {
			os.Remove(destPath)
			return "", fmt.Errorf("writing file %s: %w", destPath, copyErr)
		}

		return format, nil
	}

	return "", fmt.Errorf("no Sources index found for %s %s/%s", mirror, suite, component)
}
