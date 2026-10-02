// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/canonical/sunbeam-watchtower/internal/app"
	dto "github.com/canonical/sunbeam-watchtower/pkg/dto/v1"
)

// ConfigOutput is the response for GET /api/v1/config.
type ConfigOutput struct {
	Body *dto.Config
}

// ConfigReloadResult is the response body for POST /api/v1/config/reload.
type ConfigReloadResult struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ConfigReloadOutput is the response for POST /api/v1/config/reload.
type ConfigReloadOutput struct {
	Body *ConfigReloadResult
}

// RegisterConfigAPI registers configuration-related endpoints on the given huma API.
func RegisterConfigAPI(api huma.API, application *app.App) {
	facade := frontend.NewServerFacade(application)

	huma.Register(api, huma.Operation{
		OperationID: "config-show",
		Method:      http.MethodGet,
		Path:        "/api/v1/config",
		Summary:     "Return the loaded configuration",
		Tags:        []string{"config"},
	}, func(ctx context.Context, _ *struct{}) (*ConfigOutput, error) {
		result, err := facade.Config().Show(ctx)
		if err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return &ConfigOutput{Body: result}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "config-reload",
		Method:      http.MethodPost,
		Path:        "/api/v1/config/reload",
		Summary:     "Reload configuration from file",
		Tags:        []string{"config"},
	}, func(ctx context.Context, _ *struct{}) (*ConfigReloadOutput, error) {
		if err := facade.Config().Reload(ctx); err != nil {
			result := &ConfigReloadResult{
				Status:  "error",
				Message: err.Error(),
			}
			return &ConfigReloadOutput{Body: result}, nil //nolint:nilerr // intentional: report reload errors in body, not HTTP status
		}
		return &ConfigReloadOutput{Body: &ConfigReloadResult{
			Status: "ok",
		}}, nil
	})
}
