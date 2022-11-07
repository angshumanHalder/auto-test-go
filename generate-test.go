package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/alexflint/go-arg"
	"golang.org/x/exp/slices"
)

var fileNames = [...]string{"generate-test.go", "go.mod", "go.sum", "_test.go"}
var excludeDirs = [...]string{".git"}

type FNInfo struct {
	Name    string
	Inputs  []string
	Outputs []string
}

type FunctionNames struct {
	DirPath  string
	Filename string
	PkgName  string
	FnNames  []FNInfo
}

type VisitorFunc func(n ast.Node) ast.Visitor

func (f VisitorFunc) Visit(n ast.Node) ast.Visitor {
	return f(n)
}

func FindDirs(path string) {
	dirs, err := os.ReadDir(path)
	if err != nil {
		log.Fatalf("unable to read dirs on current path")
	}
OUTER:
	for _, entry := range dirs {
		nextPath := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			for _, dir := range excludeDirs {
				if entry.Name() == dir {
					continue OUTER
				}
			}
			FindDirs(nextPath)
		} else {
			fileName := filepath.Base(nextPath)
			if fileInExceptionList(fileName) {
				continue
			}
			functionNames := findFunctions(nextPath, path)
			generateTestFile(functionNames)
		}
	}
}

func generateTestFile(fnNames FunctionNames) {
	testFileName := strings.TrimSuffix(fnNames.Filename, filepath.Ext(fnNames.Filename)) + "_test.go"
	var appendToFile bool
	if _, err := os.Stat(testFileName); err == nil {
		fnInfos := appendNewFunctionsToFile(testFileName, fnNames.FnNames)
		appendToFile = true
		fnNames.FnNames = fnInfos
	}
	createTestFile(testFileName, fnNames, appendToFile)
}

func appendNewFunctionsToFile(testFileName string, fnNames []FNInfo) []FNInfo {
	functionNames := findFunctions(testFileName, "")
	var fnInfos []FNInfo
	for _, fn := range fnNames {
		idx := slices.IndexFunc(functionNames.FnNames, func(c FNInfo) bool { return c.Name == fmt.Sprintf("Test_%s_%s", functionNames.PkgName, fn.Name) })
		if idx == -1 {
			fnInfos = append(fnInfos, fn)
		}
	}
	return fnInfos
}

func createTestFile(testFileName string, fnNames FunctionNames, appendToFile bool) {
	var temp *template.Template
	if !appendToFile {
		temp = template.Must(template.New("testfile").Parse(`package {{.PkgName}}

import "testing"

{{range $y, $x := .FnNames -}}

func Test_{{$.PkgName}}_{{$x.Name}}(t *testing.T) {
  tests := []struct {
    name string{{range $i,$v := $x.Inputs}}
    input{{$i}} {{$v}}{{end}}{{range $i,$v := $x.Outputs}}
    expected{{$i}} {{$v}}{{end}}
  }{}
}

{{end}}
`))
	} else {
		temp = template.Must(template.New("functions").Parse(`
{{range $y, $x := .FnNames -}}

func Test_{{$.PkgName}}_{{$x.Name}}(t *testing.T) {
  tests := []struct {
    name string{{range $i,$v := $x.Inputs}}
    input{{$i}} {{$v}}{{end}}{{range $i,$v := $x.Outputs}}
    expected{{$i}} {{$v}}{{end}}
  }{}
}

{{end}}
`))
	}
	file, err := os.OpenFile(testFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("unable to create file: %v", err)
	}
	defer file.Close()

	err = temp.Execute(file, fnNames)
	if err != nil {
		log.Fatalf("unable to insert into file: %v", err)
	}
}

func fileInExceptionList(file string) bool {
	for _, name := range fileNames {
		if strings.Contains(file, name) {
			return true
		}
	}
	return filepath.Ext(file) != ".go"
}

func findFunctions(name string, path string) FunctionNames {
	funcNames := FunctionNames{}
	funcNames.DirPath = path
	funcNames.Filename = name
	funcNames.FnNames = make([]FNInfo, 0)

	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, name, nil, 0)
	if err != nil {
		log.Fatalf("error parsing file %v", name)
	}

	findFns := func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			fnInfo := FNInfo{}
			fnInfo.Name = n.Name.Name
			for _, param := range n.Type.Params.List {
				fnInfo.Inputs = append(fnInfo.Inputs, types.ExprString(param.Type))
			}
			if n.Type.Results != nil {
				for _, result := range n.Type.Results.List {
					fnInfo.Outputs = append(fnInfo.Outputs, types.ExprString(result.Type))
				}
			}
			funcNames.FnNames = append(funcNames.FnNames, fnInfo)
		case *ast.File:
			funcNames.PkgName = n.Name.Name
		}
		return true
	}
	ast.Inspect(file, findFns)

	return funcNames
}

var args struct {
	Path string `default:"./"`
}

func main() {
	arg.MustParse(&args)
	FindDirs(args.Path)
}
