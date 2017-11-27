// Copyright 2016 The G3N Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	PROGNAME      = "g3nshaders"
	VMAJOR        = 0
	VMINOR        = 1
	SHADEREXT     = ".glsl"
	DIR_INCLUDE   = "include"
	TYPE_VERTEX   = "vertex"
	TYPE_FRAGMENT = "fragment"
	TYPE_GEOMETRY = "geometry"
)

//
// Go template to generate the output file with the shaders' sources and
// maps describing the include and shader names and programs shaders.
//
const TEMPLATE = `// File generated by G3NSHADERS. Do not edit.
// To regenerate this file install 'g3nshaders' and execute:
// 'go generate' in this folder.
package {{.Pkg}}

{{range .Includes}}
const include_{{.Name}}_source = ` + "`{{.Source}}`" + `
{{end}}

{{range .Shaders}}
const {{.Name}}_source = ` + "`{{.Source}}`" + `
{{end}}

// Maps include name with its source code
var includeMap = map[string]string {
{{range .Includes}}
	"{{- .Name}}": include_{{.Name}}_source, {{end}}
}

// Maps shader name with its source code
var shaderMap = map[string]string {
{{range .Shaders}}
	"{{- .Name}}": {{.Name}}_source, {{end}}
}

// Maps program name with Proginfo struct with shaders names
var programMap = map[string]ProgramInfo{
{{ range $progName, $progInfo := .Programs }}
	"{{$progName}}": { "{{$progInfo.Vertex}}","{{$progInfo.Fragment}}","{{$progInfo.Geometry}}" }, {{end}}
}
`

// Command line options
var (
	oVersion = flag.Bool("version", false, "Show version and exits")
	oInp     = flag.String("in", ".", "Input directory")
	oOut     = flag.String("out", "sources.go", "Go output file")
	oPackage = flag.String("pkg", "shaders", "Package name")
	oVerbose = flag.Bool("v", false, "Show files being processed")
)

// Valid shader types
var shaderTypes = map[string]bool{
	TYPE_VERTEX:   true,
	TYPE_FRAGMENT: true,
	TYPE_GEOMETRY: true,
}

// fileInfo describes a shader or include file name and source code
type fileInfo struct {
	Name    string // shader or include name
	Source  string // shader or include source code
	Include bool   // true if include, false otherwise
}

// progInfo describes all the shader names of an specific program
// If the program doesn't use the geometry shader it is set as an empty string
type progInfo struct {
	Vertex   string // vertex shader name
	Fragment string // fragment shader name
	Geometry string // geometry shader name
}

// templInfo contains all information needed for the template expansion
type templInfo struct {
	Pkg      string
	Includes []fileInfo
	Shaders  []fileInfo
	Programs map[string]progInfo
}

var templData templInfo

func main() {

	// Parse command line parameters
	flag.Usage = usage
	flag.Parse()

	// If requested, print version and exits
	if *oVersion == true {
		fmt.Fprintf(os.Stderr, "%s v%d.%d\n", PROGNAME, VMAJOR, VMINOR)
		return
	}

	// Initialize template data
	templData.Pkg = *oPackage
	templData.Programs = make(map[string]progInfo)

	// Process the current directory and its subdirectories recursively
	// appending information into templData
	processDir(*oInp, false)

	// Generates output file from TEMPLATE
	generate(*oOut)
}

// processDir processes recursively all shaders files in the specified directory
func processDir(dir string, include bool) {

	// Open directory
	f, err := os.Open(dir)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// Read all fileinfos
	finfos, err := f.Readdir(0)
	if err != nil {
		panic(err)
	}

	// Process subdirectory recursively or process file
	for _, fi := range finfos {
		if fi.IsDir() {
			dirInclude := include
			if fi.Name() == DIR_INCLUDE {
				dirInclude = true
			}
			processDir(filepath.Join(dir, fi.Name()), dirInclude)
		} else {
			processFile(filepath.Join(dir, fi.Name()), include)
		}
	}
}

// processFile process one file checking if it has the shaders extension,
// otherwise it is ignored.
// If the include flag is true the file is an include file otherwise it
// it a shader
func processFile(file string, include bool) {

	// Ignore file if it has not the shader extension
	fext := filepath.Ext(file)
	if fext != SHADEREXT {
		if *oVerbose {
			fmt.Printf("Ignored file (not shader): %s\n", file)
		}
		return
	}

	// Get the file base name and its name with the extension
	fbase := filepath.Base(file)
	fname := fbase[:len(fbase)-len(fext)]

	// If not in include directory, the file must be a shader program
	// which name must have the format: <name>_<shader_type>
	if !include {
		parts := strings.Split(string(fname), "_")
		if len(parts) < 2 {
			fmt.Printf("Ignored file (INVALID NAME): %s\n", file)
			return
		}
		stype := parts[len(parts)-1]
		if !shaderTypes[stype] {
			fmt.Printf("Ignored file (INVALID SHADER TYPE): %s\n", file)
			return
		}
		sname := strings.Join(parts[:len(parts)-1], "_")
		pinfo, ok := templData.Programs[sname]
		if !ok {
			templData.Programs[sname] = pinfo
		}
		switch stype {
		case TYPE_VERTEX:
			pinfo.Vertex = fname
		case TYPE_FRAGMENT:
			pinfo.Fragment = fname
		case TYPE_GEOMETRY:
			pinfo.Geometry = fname
		}
		templData.Programs[sname] = pinfo
	}

	// Reads all file data
	f, err := os.Open(file)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}

	// Appends entry in Includes or Shaders
	if include {
		templData.Includes = append(templData.Includes, fileInfo{
			Name:   fname,
			Source: string(data),
		})
	} else {
		templData.Shaders = append(templData.Shaders, fileInfo{
			Name:   fname,
			Source: string(data),
		})
	}
	if *oVerbose {
		fmt.Printf("%s (%v bytes)\n", file, len(data))
	}
}

// generate generates output go file with shaders sources from TEMPLATE
func generate(file string) {

	// Parses the template
	tmpl := template.New("tmpl")
	tmpl, err := tmpl.Parse(TEMPLATE)
	if err != nil {
		panic(err)
	}

	// Expands template to buffer
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, &templData)
	if err != nil {
		panic(err)
	}

	// Formats buffer as Go source
	p, err := format.Source(buf.Bytes())
	if err != nil {
		panic(err)
	}

	// Writes formatted source to output file
	f, err := os.Create(file)
	if err != nil {
		panic(err)
	}
	f.Write(p)
	f.Close()
}

// usage shows the application usage
func usage() {

	fmt.Fprintf(os.Stderr, "%s v%d.%d\n", PROGNAME, VMAJOR, VMINOR)
	fmt.Fprintf(os.Stderr, "usage: %s [options]\n", strings.ToLower(PROGNAME))
	flag.PrintDefaults()
	os.Exit(2)
}
