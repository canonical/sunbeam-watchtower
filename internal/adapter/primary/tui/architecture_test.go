// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/canonical/sunbeam-watchtower/tools/archtest"
)

const tuiClientImportPath = "github.com/canonical/sunbeam-watchtower/pkg/client"

var tuiBootstrapFiles = map[string]bool{
	"app.go": true,
}

func TestTUICodeDoesNotImportPkgClientDirectly(t *testing.T) {
	files, err := archtest.LoadGoFiles("*.go", tuiBootstrapFiles)
	if err != nil {
		t.Fatalf("LoadGoFiles() error = %v", err)
	}

	for _, file := range files {
		for _, imported := range file.AST.Imports {
			if strings.Trim(imported.Path.Value, "\"") == tuiClientImportPath {
				t.Fatalf("%s imports %q directly; use the shared runtime/bootstrap or frontend facade instead", file.Path, tuiClientImportPath)
			}
		}

		archtest.Visit[*ast.SelectorExpr](file.AST, func(selector *ast.SelectorExpr) {
			pkgIdent, ok := selector.X.(*ast.Ident)
			if !ok {
				return
			}
			if pkgIdent.Name == "client" {
				t.Fatalf("%s references client.%s directly", file.Path, selector.Sel.Name)
			}
		})
	}
}
