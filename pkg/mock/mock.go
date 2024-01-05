package mock

import (
	"bytes"
	"context"
	"fmt"
	"go/types"
	"html/template"
	log "log/slog"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/imports"
)

type Options struct {
	BaseDir        string
	SearchPackages string
	PackageName    string
	OutputDir      string

	ClientDefault bool
}

type PackageInfo struct {
	Path string
	Name string
}

type FuncSig struct {
	FuncName string
	Return   string
}

type TemplateData struct {
	ClientDefault bool
	PackageName   string
	Middlewares   map[PackageInfo][]FuncSig
}

// This is hardcoded to only look for the services clients
var filter = regexp.MustCompile("github.com/aws/aws-sdk-go-v2/service/*")

// serviceNames is a mapping of package names to 'proper' naming conventions for the service
var serviceNames = map[string]string{
	"cloudformation": "CloudFormation",
	"ec2":            "EC2",
	"sts":            "STS",
}

func Run(opts *Options) error {
	resses := make(map[PackageInfo][]FuncSig)

	conf := &packages.Config{Dir: opts.BaseDir, Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports | packages.NeedTypes | packages.NeedTypesInfo}
	pkgs, err := packages.Load(conf, strings.Split(opts.SearchPackages, ",")...)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	for _, pkg := range pkgs {
		if len(pkg.Errors) != 0 {
			fmt.Println(pkg.Errors)
			os.Exit(1)
		}
	}

	for _, pkg := range pkgs {
		for _, obj := range pkg.TypesInfo.Uses {

			// filter out all the func types
			if f, ok := obj.(*types.Func); ok {
				// some (error).Error() objects do not have a Pkg. Filter these out so .Pkg().Path() does not panic
				if obj.Pkg() == nil {
					continue
				}

				// filter out only funcs where package matches
				if filter.Match([]byte(obj.Pkg().Path())) {
					// If parent is nil it's a method
					if f.Parent() == nil {
						sig := f.Type().(*types.Signature)

						log.Debug("func", obj.Name(), obj.Pkg().Name(), obj.Pkg().Path(), pkg.Fset.Position(obj.Pos()))
						packageKey := PackageInfo{
							Name: f.Pkg().Name(),
							Path: f.Pkg().Path(),
						}

						funcSig := FuncSig{
							FuncName: f.Name(),
							Return:   strings.Split(sig.Results().At(0).Type().String(), ".")[2],
						}
						if !slices.Contains(resses[packageKey], funcSig) {
							resses[packageKey] = append(resses[packageKey], funcSig)
						}
					}
				}
			}
		}
	}

	// The template writer is useful to see what packages are found when debugging issues and only prints when debug is enabled
	if log.Default().Handler().Enabled(context.Background(), log.LevelDebug) {
		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 8, 8, 0, '\t', 0)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", "Package Name", "Path", "Func", "Return")

		for k, v := range resses {
			for _, j := range v {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", k.Name, k.Path, j.FuncName, j.Return)
			}
		}
		w.Flush()
	}

	t := TemplateData{
		ClientDefault: opts.ClientDefault,
		PackageName:   opts.PackageName,
		Middlewares:   resses,
	}

	tmpl, err := template.New("mock").Funcs(templateFuncs()).Parse(fullTemplate)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, t); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	formatted, err := imports.Process("filename", buf.Bytes(), &imports.Options{
		TabWidth:  4,
		TabIndent: true,
		Comments:  true,
		Fragment:  true,
	})
	if err != nil {
		// If something is really broke we won't be able to see the contents after formatting so it's useful
		// to dump the bytes here if we are debugging
		log.Debug(buf.String())
		log.Error(err.Error())
		os.Exit(1)
	}

	if opts.OutputDir == "" {
		fmt.Println(string(formatted))
	} else {
		if err := os.MkdirAll(opts.OutputDir, os.ModePerm); err != nil {
			return err
		}
		return os.WriteFile(path.Join(opts.OutputDir, opts.PackageName+".go"), formatted, 0644)
	}

	return nil
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"ToTitle": func(s string) string {
			if n, ok := serviceNames[s]; ok {
				return n
			}
			return cases.Title(language.English).String(s)
		},
		"FirstCharLower": func(s string) string {
			return string(strings.ToLower(s)[0])
		},
		"LowerCaseFirst": func(s string) string {
			r := []rune(s)
			r[0] = unicode.ToLower(r[0])

			return string(r)
		},
	}
}
