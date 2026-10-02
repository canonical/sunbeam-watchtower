// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package charmhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"

	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

var infoEndpoint = "https://api.charmhub.io/v2/charms/info/"

// Source fetches public charm release information from Charmhub.
type Source struct {
	client *http.Client
}

// NewSource creates a charm release source.
func NewSource(client *http.Client) *Source {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Source{client: client}
}

func (s *Source) ArtifactType() dto.ArtifactType { return dto.ArtifactCharm }

// Fetch returns the current published charm channel state with per-channel resources.
func (s *Source) Fetch(ctx context.Context, publication dto.TrackedPublication) (*dto.PublishedArtifactSnapshot, error) {
	channelMap, err := s.fetchChannelMap(ctx, publication.Name)
	if err != nil {
		return nil, err
	}
	byChannel := make(map[string]*dto.ReleaseChannelSnapshot)
	for _, entry := range channelMap.ChannelMap {
		track, risk, branch, err := dto.ParseReleaseChannelName(entry.Channel.Name)
		if err != nil {
			continue
		}
		if !publication.AllowsChannel(track, risk, branch) {
			continue
		}
		channelName := entry.Channel.Name
		channel := byChannel[channelName]
		if channel == nil {
			channel = &dto.ReleaseChannelSnapshot{
				Track:     track,
				Risk:      risk,
				Branch:    branch,
				Channel:   channelName,
				UpdatedAt: entry.Channel.ReleasedAt,
			}
			byChannel[channelName] = channel
		}
		target := dto.ReleaseTargetSnapshot{
			Architecture: entry.Channel.Base.Architecture,
			Base: dto.ReleaseBase{
				Name:         entry.Channel.Base.Name,
				Channel:      entry.Channel.Base.Channel,
				Architecture: entry.Channel.Base.Architecture,
			},
			Revision:   entry.Revision.Revision,
			Version:    entry.Revision.Version,
			ReleasedAt: entry.Channel.ReleasedAt,
		}
		channel.Targets = append(channel.Targets, target)
		if target.ReleasedAt.After(channel.UpdatedAt) {
			channel.UpdatedAt = target.ReleasedAt
		}
	}

	channelNames := make([]string, 0, len(byChannel))
	for name := range byChannel {
		channelNames = append(channelNames, name)
	}
	sort.Strings(channelNames)
	for _, channelName := range channelNames {
		resources, err := s.fetchChannelResources(ctx, publication.Name, channelName, publication.Resources)
		if err != nil {
			return nil, err
		}
		byChannel[channelName].Resources = resources
	}

	channels := make([]dto.ReleaseChannelSnapshot, 0, len(byChannel))
	for _, name := range channelNames {
		channels = append(channels, dto.NormalizeChannel(*byChannel[name]))
	}

	tracks := publication.AllTracks()
	if len(tracks) == 0 {
		for _, channel := range channels {
			tracks = append(tracks, channel.Track)
		}
	}

	return &dto.PublishedArtifactSnapshot{
		Project:      publication.Project,
		Name:         publication.Name,
		ArtifactType: dto.ArtifactCharm,
		Tracks:       tracks,
		Channels:     channels,
		UpdatedAt:    time.Now().UTC(),
	}, nil
}

func (s *Source) fetchChannelMap(ctx context.Context, name string) (*charmInfoResponse, error) {
	fields := url.Values{}
	fields.Set("fields", "channel-map")
	endpoint := infoEndpoint + url.PathEscape(name) + "?" + fields.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating charm info request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching charm info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching charm info: HTTP %d", resp.StatusCode)
	}
	var payload charmInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding charm info: %w", err)
	}
	return &payload, nil
}

func (s *Source) fetchChannelResources(ctx context.Context, name, channel string, expected []string) ([]dto.ReleaseResourceSnapshot, error) {
	fields := url.Values{}
	fields.Set("channel", channel)
	fields.Set("fields", "default-release.resources")
	endpoint := infoEndpoint + url.PathEscape(name) + "?" + fields.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating charm resource request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching charm resources: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching charm resources: HTTP %d", resp.StatusCode)
	}
	var payload charmResourcesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding charm resources: %w", err)
	}
	resources := make([]dto.ReleaseResourceSnapshot, 0, len(payload.DefaultRelease.Resources))
	expectedSet := make(map[string]bool, len(expected))
	for _, name := range expected {
		expectedSet[name] = true
	}
	for _, resource := range payload.DefaultRelease.Resources {
		if len(expectedSet) > 0 && !expectedSet[resource.Name] {
			continue
		}
		resources = append(resources, dto.ReleaseResourceSnapshot{
			Name:        resource.Name,
			Type:        resource.Type,
			Revision:    resource.Revision,
			Filename:    resource.Filename,
			Description: resource.Description,
		})
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Name < resources[j].Name
	})
	return resources, nil
}

type charmInfoResponse struct {
	ChannelMap []struct {
		Channel struct {
			Base struct {
				Architecture string `json:"architecture"`
				Channel      string `json:"channel"`
				Name         string `json:"name"`
			} `json:"base"`
			Name       string    `json:"name"`
			ReleasedAt time.Time `json:"released-at"`
			Risk       string    `json:"risk"`
			Track      string    `json:"track"`
		} `json:"channel"`
		Revision struct {
			Revision int    `json:"revision"`
			Version  string `json:"version"`
		} `json:"revision"`
	} `json:"channel-map"`
}

type charmResourcesResponse struct {
	DefaultRelease struct {
		Resources []struct {
			Name        string `json:"name"`
			Type        string `json:"type"`
			Revision    int    `json:"revision"`
			Filename    string `json:"filename"`
			Description string `json:"description"`
		} `json:"resources"`
	} `json:"default-release"`
}
