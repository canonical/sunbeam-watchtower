// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/gitcache"
	"github.com/canonical/sunbeam-watchtower/internal/core/service/artifactdiscovery"
)

// gitcacheTreeReader adapts gitcache's free functions to the discovery
// service's TreeReader port.
type gitcacheTreeReader struct{}

func (gitcacheTreeReader) ReadHEADFile(repoPath, filePath string) ([]byte, error) {
	return gitcache.ReadHEADFile(repoPath, filePath)
}

func (gitcacheTreeReader) FindHEADFilesByBaseName(repoPath, baseName string) ([]artifactdiscovery.HeadFile, error) {
	files, err := gitcache.FindHEADFilesByBaseName(repoPath, baseName)
	if err != nil {
		return nil, err
	}
	out := make([]artifactdiscovery.HeadFile, len(files))
	for i, f := range files {
		out[i] = artifactdiscovery.HeadFile{Path: f.Path, Content: f.Content}
	}
	return out, nil
}

// ArtifactDiscoveryService returns a lazily initialized artifact discovery
// service backed by the gitcache HEAD tree reader.
func (a *App) ArtifactDiscoveryService() (*artifactdiscovery.Service, error) {
	a.artifactDiscoveryOnce.Do(func() {
		a.artifactDiscoverySvc = artifactdiscovery.NewService(gitcacheTreeReader{}, a.Logger)
	})
	return a.artifactDiscoverySvc, nil
}
