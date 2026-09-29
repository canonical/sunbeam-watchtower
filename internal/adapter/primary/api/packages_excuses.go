// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/gboutry/sunbeam-watchtower/internal/app"
	pkg "github.com/gboutry/sunbeam-watchtower/internal/core/service/package"
	dto "github.com/gboutry/sunbeam-watchtower/pkg/dto/v1"
)

// PackagesExcusesListInput holds parameters for the excuses list endpoint.
type PackagesExcusesListInput struct {
	Trackers     []string `query:"tracker" required:"false" doc:"Excuses tracker(s) to query (default: configured default tracker)"`
	Name         string   `query:"name" required:"false" doc:"Case-insensitive regex to filter source package names"`
	Component    string   `query:"component" required:"false" doc:"Archive component filter"`
	Team         string   `query:"team" required:"false" doc:"Owning team filter"`
	FTBFS        bool     `query:"ftbfs" required:"false" doc:"Only show FTBFS excuses"`
	Autopkgtest  bool     `query:"autopkgtest" required:"false" doc:"Only show excuses involving autopkgtests"`
	BlockedBy    string   `query:"blocked_by" required:"false" doc:"Only show excuses blocked by this package"`
	Bugged       bool     `query:"bugged" required:"false" doc:"Only show excuses with an attached bug reference"`
	MinAge       int      `query:"min_age" required:"false" doc:"Only include excuses at least this many days old"`
	MaxAge       int      `query:"max_age" required:"false" doc:"Only include excuses no older than this many days"`
	Limit        int      `query:"limit" required:"false" doc:"Maximum number of results to return"`
	Reverse      bool     `query:"reverse" required:"false" doc:"Show older excuses first"`
	Set          string   `query:"set" required:"false" doc:"Only show excuses for packages in the named config set"`
	BlockedBySet string   `query:"blocked_by_set" required:"false" doc:"Only show excuses blocked by packages in the named config set"`
}

// PackagesExcusesListOutput is the response for the excuses list endpoint.
type PackagesExcusesListOutput struct {
	Body []dto.PackageExcuseSummary `doc:"List of package excuses"`
}

// PackagesExcusesShowInput holds parameters for the excuses show endpoint.
type PackagesExcusesShowInput struct {
	Name    string `path:"name" doc:"Source package name" example:"nova"`
	Tracker string `query:"tracker" required:"false" doc:"Excuses tracker to query (default: configured default tracker)"`
	Version string `query:"version" required:"false" doc:"Exact Debian version string"`
}

// PackagesExcusesShowOutput is the response for the excuses show endpoint.
type PackagesExcusesShowOutput struct {
	Body *dto.PackageExcuse `doc:"Detailed package excuse"`
}

func registerPackagesExcusesAPI(api huma.API, application *app.App) {
	huma.Register(api, huma.Operation{
		OperationID: "packages-excuses-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/packages/excuses",
		Summary:     "List package migration excuses",
		Tags:        []string{"packages"},
	}, func(ctx context.Context, input *PackagesExcusesListInput) (*PackagesExcusesListOutput, error) {
		sources := application.ExcusesSources()
		trackers := input.Trackers
		if len(trackers) == 0 {
			if len(sources) == 0 {
				return nil, huma.Error400BadRequest("no excuses trackers configured")
			}
			trackers = []string{application.DefaultExcusesTracker()}
		}
		if err := dto.ValidateExcusesTrackers(sources, trackers); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		var packages, blockedByPackages []string
		if input.Set != "" {
			pkgs, err := application.PackageSet(input.Set)
			if err != nil {
				return nil, packageSetHTTPError(err)
			}
			packages = pkgs
		}
		if input.BlockedBySet != "" {
			pkgs, err := application.PackageSet(input.BlockedBySet)
			if err != nil {
				return nil, packageSetHTTPError(err)
			}
			blockedByPackages = pkgs
		}

		cache, err := application.ExcusesCache()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to open excuses cache: %v", err))
		}
		svc := pkg.NewExcusesService(cache, application.Logger)
		results, err := svc.List(ctx, dto.ExcuseQueryOpts{
			Trackers:          trackers,
			Name:              input.Name,
			Component:         input.Component,
			Team:              input.Team,
			FTBFS:             input.FTBFS,
			Autopkgtest:       input.Autopkgtest,
			BlockedBy:         input.BlockedBy,
			Bugged:            input.Bugged,
			MinAge:            input.MinAge,
			MaxAge:            input.MaxAge,
			Limit:             input.Limit,
			Reverse:           input.Reverse,
			Packages:          packages,
			BlockedByPackages: blockedByPackages,
		})
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("listing excuses failed: %v", err))
		}
		return &PackagesExcusesListOutput{Body: results}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "packages-excuses-show",
		Method:      http.MethodGet,
		Path:        "/api/v1/packages/excuses/{name}",
		Summary:     "Show one package migration excuse",
		Tags:        []string{"packages"},
	}, func(ctx context.Context, input *PackagesExcusesShowInput) (*PackagesExcusesShowOutput, error) {
		sources := application.ExcusesSources()
		tracker := input.Tracker
		if tracker == "" {
			if len(sources) == 0 {
				return nil, huma.Error400BadRequest("no excuses trackers configured")
			}
			tracker = application.DefaultExcusesTracker()
		}
		if err := dto.ValidateExcusesTrackers(sources, []string{tracker}); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		cache, err := application.ExcusesCache()
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to open excuses cache: %v", err))
		}
		svc := pkg.NewExcusesService(cache, application.Logger)
		result, err := svc.Show(ctx, tracker, input.Name, input.Version)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		return &PackagesExcusesShowOutput{Body: result}, nil
	})
}
