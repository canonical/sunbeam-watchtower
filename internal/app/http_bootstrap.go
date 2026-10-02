// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net/http"
	"time"

	oteladapter "github.com/canonical/sunbeam-watchtower/internal/adapter/secondary/otel"
)

func (a *App) upstreamHTTPClient(upstream string, timeout time.Duration) *http.Client {
	return oteladapter.WrapHTTPClient(&http.Client{Timeout: timeout}, upstream)
}
