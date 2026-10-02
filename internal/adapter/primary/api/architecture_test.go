// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/canonical/sunbeam-watchtower/tools/archtest"
)

var apiFacadeExemptFiles = map[string]bool{
	"api_architecture_helpers.go": true,
	"cache.go":                    true,
	"packages.go":                 true,
	"packages_excuses.go":         true,
	"server.go":                   true,
}

func TestAPIHandlersUseServerFacadeInsteadOfAppSelectors(t *testing.T) {
	files, err := archtest.LoadGoFiles("*.go", apiFacadeExemptFiles)
	if err != nil {
		t.Fatalf("LoadGoFiles() error = %v", err)
	}

	for _, file := range files {
		archtest.Visit[*ast.SelectorExpr](file.AST, func(selector *ast.SelectorExpr) {
			ident, ok := selector.X.(*ast.Ident)
			if ok && ident.Name == "application" {
				t.Fatalf("%s uses application.%s directly; route API handler logic through frontend.NewServerFacade()", file.Path, selector.Sel.Name)
			}
		})
	}
}

func TestAPIHandlersDoNotInstantiateWorkflowsDirectly(t *testing.T) {
	files, err := archtest.LoadGoFiles("*.go", apiFacadeExemptFiles)
	if err != nil {
		t.Fatalf("LoadGoFiles() error = %v", err)
	}

	for _, file := range files {
		archtest.Visit[*ast.CallExpr](file.AST, func(call *ast.CallExpr) {
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return
			}

			pkgIdent, ok := selector.X.(*ast.Ident)
			if !ok || pkgIdent.Name != "frontend" {
				return
			}

			if strings.HasPrefix(selector.Sel.Name, "New") && selector.Sel.Name != "NewServerFacade" {
				t.Fatalf("%s calls frontend.%s directly; API handlers must use frontend.NewServerFacade()", file.Path, selector.Sel.Name)
			}
		})
	}
}
