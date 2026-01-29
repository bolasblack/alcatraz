// Package fslint provides a linter that detects direct filesystem operations
// in specified packages (except allowed packages specified in config).
package fslint

import (
	"fmt"
	"go/ast"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/tools/go/analysis"
)

var configFile string

// Config represents the fslint configuration.
type Config struct {
	ScanDirs        []string            `toml:"scan_dirs"`
	AllowedPackages []string            `toml:"allowed_packages"`
	ForbiddenCalls  map[string][]string `toml:"forbidden_calls"`
}

// Analyzer is the fslint analyzer.
var Analyzer = &analysis.Analyzer{
	Name: "fslint",
	Doc:  "detects direct filesystem operations in specified packages (except allowed packages specified in config)",
	Run:  run,
}

func init() {
	Analyzer.Flags.StringVar(&configFile, "config", "", "path to fslint config file (required)")
}

func loadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, fmt.Errorf("config file path is required (use -config flag)")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

func run(pass *analysis.Pass) (interface{}, error) {
	cfg, err := loadConfig(configFile)
	if err != nil {
		return nil, err
	}

	pkgPath := pass.Pkg.Path()

	// Skip if not in scan_dirs
	if !shouldScanPackage(pkgPath, cfg.ScanDirs) {
		return nil, nil
	}

	// Skip if in allowed packages
	if isAllowedPackage(pkgPath, cfg.AllowedPackages) {
		return nil, nil
	}

	// Build forbidden functions map for quick lookup
	forbiddenFuncs := make(map[string]map[string]bool)
	for pkg, funcs := range cfg.ForbiddenCalls {
		forbiddenFuncs[pkg] = make(map[string]bool)
		for _, fn := range funcs {
			forbiddenFuncs[pkg][fn] = true
		}
	}

	// Check each file
	for _, file := range pass.Files {
		imports := buildImportMap(file)

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			// Get the package path from the import alias
			importPath, ok := imports[ident.Name]
			if !ok {
				return true
			}

			// Check if this is a forbidden function call
			funcName := sel.Sel.Name
			if funcs, ok := forbiddenFuncs[importPath]; ok {
				if funcs[funcName] {
					pass.Reportf(call.Pos(), "direct filesystem operation %s.%s is not allowed in this package (use env.Fs instead)", ident.Name, funcName)
				}
			}

			return true
		})
	}

	return nil, nil
}

// shouldScanPackage checks if the package path should be scanned based on scan_dirs config.
func shouldScanPackage(pkgPath string, scanDirs []string) bool {
	for _, dir := range scanDirs {
		// Match: contains /dir or starts with dir
		if strings.Contains(pkgPath, "/"+dir) || strings.HasPrefix(pkgPath, dir) {
			return true
		}
	}
	return false
}

// isAllowedPackage checks if pkgPath matches any allowed package or is a subpackage of it.
func isAllowedPackage(pkgPath string, allowedPackages []string) bool {
	for _, allowed := range allowedPackages {
		if matchesPackagePath(pkgPath, allowed) {
			return true
		}
	}
	return false
}

// matchesPackagePath checks if pkgPath matches the pattern or is a subpackage of it.
// Pattern examples: "internal/transact", "internal/sudo"
func matchesPackagePath(pkgPath, pattern string) bool {
	// Exact match: github.com/foo/internal/transact == internal/transact suffix
	// Subpackage match: github.com/foo/internal/transact/sub contains /internal/transact/
	return strings.HasSuffix(pkgPath, "/"+pattern) ||
		strings.Contains(pkgPath, "/"+pattern+"/") ||
		pkgPath == pattern ||
		strings.HasPrefix(pkgPath, pattern+"/")
}

// buildImportMap builds a map from import alias to package path.
func buildImportMap(file *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		var name string
		if imp.Name != nil {
			name = imp.Name.Name
		} else {
			// Use the last component of the path as the default name
			parts := strings.Split(path, "/")
			name = parts[len(parts)-1]
		}
		imports[name] = path
	}
	return imports
}
