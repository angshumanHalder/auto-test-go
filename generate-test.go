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

	"golang.org/x/exp/slices"
)

var fileNames = [...]string{"generate-test.go", "go.mod", "go.sum", "_test.go"}
var fileExts = [...]string{".txt", ".yml", ".env"}

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

func main() {
	findDirs("./")
}

func findDirs(path string) {
	dirs, err := os.ReadDir(path)
	if err != nil {
		log.Fatalf("unable to read dirs on current path")
	}
	for _, entry := range dirs {
		nextPath := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			findDirs(nextPath)
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
	if appendToFile {
		tmp, err := template.ParseFiles("functions.txt")
		if err != nil {
			log.Fatalf("unable to parse template file: %v", err)
		}
		temp = tmp
	} else {
		tmp, err := template.ParseFiles("template.txt")
		if err != nil {
			log.Fatalf("unable to parse template file: %v", err)
		}
		temp = tmp
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
	for _, ext := range fileExts {
		if filepath.Ext(file) == ext {
			return true
		}
	}
	return false
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
			log.Println("file name, ", n.Name.Name)
			funcNames.PkgName = n.Name.Name
		}
		return true
	}
	ast.Inspect(file, findFns)

	return funcNames
}
