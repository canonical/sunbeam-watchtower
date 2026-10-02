// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/canonical/sunbeam-watchtower/tools/archtest"
)

const clientImportPath = "github.com/canonical/sunbeam-watchtower/pkg/client"

var cliBootstrapFiles = map[string]bool{
	"root.go":    true,
	"runtime.go": true,
}

func TestCommandFilesDoNotImportOrCallPkgClientDirectly(t *testing.T) {
	files, err := archtest.LoadGoFiles("*.go", cliBootstrapFiles)
	if err != nil {
		t.Fatalf("LoadGoFiles() error = %v", err)
	}

	for _, file := range files {
		for _, imported := range file.AST.Imports {
			if strings.Trim(imported.Path.Value, "\"") == clientImportPath {
				t.Fatalf("%s imports %q directly; route command logic through internal/adapter/primary/frontend instead", file.Path, clientImportPath)
			}
		}

		archtest.Visit[*ast.CallExpr](file.AST, func(call *ast.CallExpr) {
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return
			}

			clientSelector, ok := selector.X.(*ast.SelectorExpr)
			if !ok {
				return
			}

			optsIdent, ok := clientSelector.X.(*ast.Ident)
			if !ok {
				return
			}

			if optsIdent.Name == "opts" && clientSelector.Sel.Name == "Client" {
				t.Fatalf("%s calls opts.Client.%s directly; command handlers must delegate to frontend workflows", file.Path, selector.Sel.Name)
			}
		})
	}
}

func TestCommandFilesDoNotInstantiateFrontendWorkflowsDirectly(t *testing.T) {
	files, err := archtest.LoadGoFiles("*.go", cliBootstrapFiles)
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

			if strings.HasPrefix(selector.Sel.Name, "New") &&
				(strings.HasSuffix(selector.Sel.Name, "Workflow") || selector.Sel.Name == "NewClientFacade") {
				t.Fatalf("%s calls frontend.%s directly; command handlers must use opts.Frontend()", file.Path, selector.Sel.Name)
			}
		})
	}
}
