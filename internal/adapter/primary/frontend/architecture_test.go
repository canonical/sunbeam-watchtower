// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"

	"github.com/canonical/sunbeam-watchtower/tools/archtest"
)

const frontendClientImportPath = "github.com/canonical/sunbeam-watchtower/pkg/client"

func TestExportedFrontendAPIsDoNotExposePkgClientTypes(t *testing.T) {
	files, err := archtest.LoadGoFiles("*.go", map[string]bool{"transport.go": true})
	if err != nil {
		t.Fatalf("LoadGoFiles() error = %v", err)
	}

	for _, file := range files {
		aliases := archtest.ImportAliases(file.AST, frontendClientImportPath, "client")
		if len(aliases) == 0 {
			continue
		}

		for _, decl := range file.AST.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if !decl.Name.IsExported() || strings.HasPrefix(decl.Name.Name, "New") {
					continue
				}
				if archtest.FuncTypeUsesImport(decl.Type, aliases) {
					t.Fatalf("%s exports %s with a pkg/client type in its signature; keep exported frontend APIs transport-agnostic", file.Path, decl.Name.Name)
				}
			case *ast.GenDecl:
				if decl.Tok != token.TYPE {
					continue
				}
				for _, spec := range decl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || !typeSpec.Name.IsExported() {
						continue
					}
					if archtest.ExportedTypeUsesImport(typeSpec.Type, aliases) {
						t.Fatalf("%s exports type %s with a pkg/client field or alias; keep exported frontend DTOs transport-agnostic", file.Path, typeSpec.Name.Name)
					}
				}
			}
		}
	}
}

func TestFrontendUsesConcretePkgClientOnlyInTransportWrapper(t *testing.T) {
	files, err := archtest.LoadGoFiles("*.go", map[string]bool{"transport.go": true})
	if err != nil {
		t.Fatalf("LoadGoFiles() error = %v", err)
	}

	for _, file := range files {
		aliases := archtest.ImportAliases(file.AST, frontendClientImportPath, "client")
		if len(aliases) == 0 {
			continue
		}

		archtest.Visit[*ast.SelectorExpr](file.AST, func(selector *ast.SelectorExpr) {
			pkgIdent, ok := selector.X.(*ast.Ident)
			if !ok {
				return
			}

			if _, exists := aliases[pkgIdent.Name]; exists && selector.Sel.Name == "Client" {
				t.Fatalf("%s references the concrete pkg/client.Client type directly; route frontend wiring through transport.go", file.Path)
			}
		})
	}
}
