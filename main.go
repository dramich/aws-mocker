package main

import (
	"flag"
	"fmt"
	log "log/slog"
	"os"
	"strings"

	"github.com/dramich/aws-mocker/pkg/mock"
)

func main() {
	var (
		mockOpts mock.Options
		logLevel string
	)

	flag.StringVar(&mockOpts.BaseDir, "dir", "", "Base directory for the module")
	flag.StringVar(&mockOpts.OutputDir, "output-dir", "", "Output directory for the generated file, if not provided will write to stdout")
	flag.StringVar(&mockOpts.PackageName, "package-name", "awsmocked", "Name of the generated package")
	flag.StringVar(&mockOpts.SearchPackages, "packages", "", "Comma seperated list of packages to search")
	flag.BoolVar(&mockOpts.ClientDefault, "default-panic", false, "Add a panic for Operations that are not mocked")

	flag.StringVar(&logLevel, "log-level", "info", "Set the log level [debug, info, warn, error]")

	flag.Parse()

	log.SetDefault(log.New(log.NewTextHandler(os.Stderr, &log.HandlerOptions{
		Level: logLevelFromArg(logLevel),
	})))

	if mockOpts.SearchPackages == "" || mockOpts.BaseDir == "" {
		fmt.Println("'packages' and 'dir' are required flags")
		flag.Usage()
		os.Exit(1)
	}

	err := mock.Run(&mockOpts)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

}

func logLevelFromArg(arg string) log.Level {
	switch strings.ToLower(arg) {
	case "debug":
		return log.LevelDebug
	case "info":
		return log.LevelInfo
	case "warn":
		return log.LevelWarn
	case "error":
		return log.LevelError
	default:
		log.Warn("Unable to parse log level, defaulting to 'Info'")
		return log.LevelInfo
	}
}
