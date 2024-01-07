package mock

import (
	"bytes"
	"context"
	"fmt"
	"go/types"
	"html/template"
	"io"
	log "log/slog"
	"os"
	"regexp"
	"slices"
	"sort"
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
	Writer        io.Writer
}

type PackageInfo struct {
	Path     string
	Name     string
	FuncSigs []FuncSig
}

type FuncSig struct {
	FuncName string
	Return   string
}

type TemplateData struct {
	ClientDefault bool
	PackageName   string
	Middlewares   []PackageInfo
}

// This is hardcoded to only look for the services clients
var filter = regexp.MustCompile("github.com/aws/aws-sdk-go-v2/service/*")

// serviceNames is a mapping of package names to 'proper' naming conventions for the service
var serviceNames = map[string]string{
	"cloudformation": "CloudFormation",
	"dynamodb":       "DynamoDB",
	"ec2":            "EC2",
	"sts":            "STS",
}

func Run(opts *Options) error {
	resses := make(map[string]PackageInfo)

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
		for _, obj := range pkg.TypesInfo.Uses {
			// filter out all the func types
			if f, ok := obj.(*types.Func); ok {
				// some (error).Error() objects do not have a Pkg. Filter these out so .Pkg().Path() does not panic
				if obj.Pkg() == nil {
					continue
				}

				// filter out only funcs where package matches
				if filter.MatchString(obj.Pkg().Path()) {
					// If parent is nil it's a method
					if f.Parent() == nil {
						log.Debug("func", obj.Name(), obj.Pkg().Name(), obj.Pkg().Path(), pkg.Fset.Position(obj.Pos()))

						sig, sigOK := f.Type().(*types.Signature)
						if !sigOK {
							log.Error("failed to convert", "func", f.Name())
							os.Exit(1)
						}

						funcSig := FuncSig{
							FuncName: f.Name(),
							Return:   strings.Split(sig.Results().At(0).Type().String(), ".")[2],
						}

						if p, pkgOK := resses[f.Pkg().Path()]; pkgOK {
							if !slices.Contains(p.FuncSigs, funcSig) {
								p.FuncSigs = append(p.FuncSigs, funcSig)
								resses[f.Pkg().Path()] = p
							}
						} else {
							resses[f.Pkg().Path()] = PackageInfo{
								Name:     f.Pkg().Name(),
								Path:     f.Pkg().Path(),
								FuncSigs: []FuncSig{funcSig},
							}
						}
					}
				}
			}
		}
	}

	// The template writer is useful to see what packages are found when debugging issues and only prints when debug is enabled
	if log.Default().Handler().Enabled(context.Background(), log.LevelDebug) {
		writePackageTable(resses)
	}

	sorted := sortPackages(resses)

	t := TemplateData{
		ClientDefault: opts.ClientDefault,
		PackageName:   opts.PackageName,
		Middlewares:   sorted,
	}

	tmpl, err := template.New("mock").Funcs(templateFuncs()).Parse(fullTemplate)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, t); err != nil {
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

	_, err = opts.Writer.Write(formatted)

	return err
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

// sortPackages sorts the package based on the path, funcs based on their name
// and converts to a slice for the template
func sortPackages(in map[string]PackageInfo) []PackageInfo {
	out := make([]PackageInfo, 0, len(in))
	for _, v := range in {
		sort.Slice(v.FuncSigs, func(i, j int) bool {
			return v.FuncSigs[i].FuncName < v.FuncSigs[j].FuncName
		})
		out = append(out, v)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out
}

func writePackageTable(resses map[string]PackageInfo) {
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 8, 8, 0, '\t', 0)

	fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", "Package Name", "Path", "Func", "Return")

	for _, v := range resses {
		for _, j := range v.FuncSigs {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", v.Name, v.Path, j.FuncName, j.Return)
		}
	}
	w.Flush()
}
