// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"
	"net/url"
)

// GetArchive fetches an archive (PPA) by distribution and name.
// Path: /<distribution>/+archive/<name>
func (c *Client) GetArchive(ctx context.Context, distribution, name string) (Archive, error) {
	var a Archive
	path := fmt.Sprintf("/%s/+archive/%s", distribution, name)
	if err := c.GetJSON(ctx, path, &a); err != nil {
		return Archive{}, fmt.Errorf("fetching archive %s/%s: %w", distribution, name, err)
	}
	return a, nil
}

// GetArchiveByLink fetches an archive using its self_link.
func (c *Client) GetArchiveByLink(ctx context.Context, selfLink string) (Archive, error) {
	var a Archive
	if err := c.GetJSON(ctx, selfLink, &a); err != nil {
		return Archive{}, fmt.Errorf("fetching archive: %w", err)
	}
	return a, nil
}

// GetPublishedSources returns published source packages in an archive.
func (c *Client) GetPublishedSources(ctx context.Context, archiveSelfLink string, opts PublishedSourceOpts) ([]SourcePublishing, error) {
	params := opts.values()
	u := wsOpURL(archiveSelfLink, "getPublishedSources", params)
	return GetAllPages[SourcePublishing](ctx, c, u)
}

// GetPackageUploads reads queue entries for one Ubuntu series and source.
func (c *Client) GetPackageUploads(ctx context.Context, seriesLink, source, status string) ([]PackageUpload, error) {
	params := url.Values{"name": {source}, "exact_match": {"true"}, "pocket": {"Proposed"}}
	if status != "" {
		params.Set("status", status)
	}
	u := wsOpURL(seriesLink, "getPackageUploads", params)
	return GetAllPages[PackageUpload](ctx, c, u)
}

// GetPublicationChangesURL resolves the changes file for a source publication.
func (c *Client) GetPublicationChangesURL(ctx context.Context, publicationLink string) (string, error) {
	var raw string
	err := c.GetJSON(ctx, wsOpURL(publicationLink, "changesFileUrl", nil), &raw)
	return raw, err
}

// PublishedSourceOpts holds optional filters for getPublishedSources.
type PublishedSourceOpts struct {
	SourceName       string
	Version          string
	Status           string // Pending, Published, Superseded, Deleted, Obsolete
	Pocket           string // Release, Security, Updates, Proposed, Backports
	DistroSeries     string // link to distro_series
	ComponentName    string
	ExactMatch       bool
	OrderByDate      bool
	CreatedSinceDate string
}

func (o PublishedSourceOpts) values() url.Values {
	v := url.Values{}
	if o.SourceName != "" {
		v.Set("source_name", o.SourceName)
	}
	if o.Version != "" {
		v.Set("version", o.Version)
	}
	if o.Status != "" {
		v.Set("status", o.Status)
	}
	if o.Pocket != "" {
		v.Set("pocket", o.Pocket)
	}
	if o.DistroSeries != "" {
		v.Set("distro_series", o.DistroSeries)
	}
	if o.ComponentName != "" {
		v.Set("component_name", o.ComponentName)
	}
	if o.ExactMatch {
		v.Set("exact_match", "true")
	}
	if o.OrderByDate {
		v.Set("order_by_date", "true")
	}
	if o.CreatedSinceDate != "" {
		utc, err := mustBeUTC(o.CreatedSinceDate)
		if err != nil {
			v.Set("created_since_date", o.CreatedSinceDate)
		} else {
			v.Set("created_since_date", utc)
		}
	}
	return v
}

// GetPublishedBinaries returns published binary packages in an archive.
func (c *Client) GetPublishedBinaries(ctx context.Context, archiveSelfLink string, opts PublishedBinaryOpts) ([]BinaryPublishing, error) {
	params := opts.values()
	u := wsOpURL(archiveSelfLink, "getPublishedBinaries", params)
	return GetAllPages[BinaryPublishing](ctx, c, u)
}

// PublishedBinaryOpts holds optional filters for getPublishedBinaries.
type PublishedBinaryOpts struct {
	BinaryName       string
	Version          string
	Status           string
	Pocket           string
	DistroArchSeries string // link
	ComponentName    string
	ExactMatch       bool
	OrderByDate      bool
	CreatedSinceDate string
}

func (o PublishedBinaryOpts) values() url.Values {
	v := url.Values{}
	if o.BinaryName != "" {
		v.Set("binary_name", o.BinaryName)
	}
	if o.Version != "" {
		v.Set("version", o.Version)
	}
	if o.Status != "" {
		v.Set("status", o.Status)
	}
	if o.Pocket != "" {
		v.Set("pocket", o.Pocket)
	}
	if o.DistroArchSeries != "" {
		v.Set("distro_arch_series", o.DistroArchSeries)
	}
	if o.ComponentName != "" {
		v.Set("component_name", o.ComponentName)
	}
	if o.ExactMatch {
		v.Set("exact_match", "true")
	}
	if o.OrderByDate {
		v.Set("order_by_date", "true")
	}
	if o.CreatedSinceDate != "" {
		utc, err := mustBeUTC(o.CreatedSinceDate)
		if err != nil {
			v.Set("created_since_date", o.CreatedSinceDate)
		} else {
			v.Set("created_since_date", utc)
		}
	}
	return v
}

// GetBuildCounters returns build counters for an archive.
func (c *Client) GetBuildCounters(ctx context.Context, archiveSelfLink string) (BuildCounters, error) {
	u := wsOpURL(archiveSelfLink, "getBuildCounters", nil)
	var bc BuildCounters
	if err := c.GetJSON(ctx, u, &bc); err != nil {
		return BuildCounters{}, fmt.Errorf("fetching build counters: %w", err)
	}
	return bc, nil
}

// GetBuildRecords returns build records for an archive.
func (c *Client) GetBuildRecords(ctx context.Context, archiveSelfLink string, buildState, pocket, sourceName string) ([]Build, error) {
	params := url.Values{}
	if buildState != "" {
		params.Set("build_state", buildState)
	}
	if pocket != "" {
		params.Set("pocket", pocket)
	}
	if sourceName != "" {
		params.Set("source_name", sourceName)
	}
	u := wsOpURL(archiveSelfLink, "getBuildRecords", params)
	return GetAllPages[Build](ctx, c, u)
}

// Build represents a source package build record.
type Build struct {
	SelfLink             string `json:"self_link"`
	WebLink              string `json:"web_link"`
	ResourceTypeLink     string `json:"resource_type_link"`
	HTTPEtag             string `json:"http_etag"`
	Title                string `json:"title"`
	ArchTag              string `json:"arch_tag"`
	BuildState           string `json:"buildstate"`
	BuildLogURL          string `json:"build_log_url"`
	UploadLogURL         string `json:"upload_log_url"`
	BuilderLink          string `json:"builder_link"`
	ArchiveLink          string `json:"archive_link"`
	DistroArchSeriesLink string `json:"distro_arch_series_link"`
	Duration             string `json:"duration"`
	DateCreated          *Time  `json:"datecreated,omitempty"`
	DateStarted          *Time  `json:"date_started,omitempty"`
	DateBuilt            *Time  `json:"datebuilt,omitempty"`
	DateFirstDispatched  *Time  `json:"date_first_dispatched,omitempty"`
	CanBeRetried         bool   `json:"can_be_retried"`
	CanBeRescored        bool   `json:"can_be_rescored"`
	CanBeCancelled       bool   `json:"can_be_cancelled"`
}
