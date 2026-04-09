package parse

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/NWACus/weather-telemetry/parsers/crbasic"
	"github.com/NWACus/weather-telemetry/parsers/data"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func DefaultOptions() *RawOptions {
	return &RawOptions{}
}

func BindOptions(opts *RawOptions, cmd *cobra.Command) error {
	cmd.Flags().StringVar(&opts.DataDir, "data-dir", opts.DataDir, "Path to directory holding raw data.")
	return nil
}

// RawOptions holds input values.
type RawOptions struct {
	DataDir string
}

// validatedOptions is a private wrapper that enforces a call of Validate() before Complete() can be invoked.
type validatedOptions struct {
	*RawOptions
}

type ValidatedOptions struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*validatedOptions
}

// completedOptions is a private wrapper that enforces a call of Complete() before config generation can be invoked.
type completedOptions struct {
	DataDir string
}

type Options struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedOptions
}

func (o *RawOptions) Validate() (*ValidatedOptions, error) {
	for _, item := range []struct {
		flag  string
		name  string
		value *string
	}{
		{flag: "data-dir", name: "raw data directory", value: &o.DataDir},
	} {
		if item.value == nil || *item.value == "" {
			return nil, fmt.Errorf("the %s must be provided with --%s", item.name, item.flag)
		}
	}

	return &ValidatedOptions{
		validatedOptions: &validatedOptions{
			RawOptions: o,
		},
	}, nil
}

func (o *ValidatedOptions) Complete(ctx context.Context) (*Options, error) {
	return &Options{
		completedOptions: &completedOptions{
			DataDir: o.DataDir,
		},
	}, nil
}

var nameRegex = regexp.MustCompile("(	[a-zA-Z]{3})([0-9]{4})")

// parseName parses a file name like ALL0708.DAT into two parts - the station ID (ALL) and the year (2007)
func parseName(path string) (string, int, error) {
	matches := nameRegex.FindStringSubmatch(path)
	if len(matches) != 3 {
		return "", 0, fmt.Errorf("invalid name format: %s", path)
	}
	if len(matches[2]) != 4 {
		return "", 0, fmt.Errorf("invalid name format: %s", path)
	}
	year, err := strconv.ParseInt(matches[2][0:2], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid year format: %s", path)
	}
	return matches[1], 2000 + int(year), nil
}

func (opts *Options) Run(ctx context.Context) error {
	logger, err := logr.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	tableDefinitions := map[string]map[int][]crbasic.DataTableConfiguration{} // id -> year -> tables
	dataFiles := map[string][]string{}                                        // id -> file paths

	if err := fs.WalkDir(os.DirFS(opts.DataDir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		thisLogger := logger.WithValues("path", path)
		if strings.ToLower(filepath.Ext(path)) == ".cr1" {
			stationId, year, err := parseName(path)
			if err != nil {
				thisLogger.V(4).Error(err, "failed to parse name")
				return nil
			}
			if _, ok := tableDefinitions[stationId]; !ok {
				tableDefinitions[stationId] = map[int][]crbasic.DataTableConfiguration{}
			}
			if _, ok := tableDefinitions[stationId][year]; !ok {
				tableDefinitions[stationId][year] = []crbasic.DataTableConfiguration{}
			}

			raw, err := os.ReadFile(filepath.Join(opts.DataDir, path))
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", path, err)
			}

			cfg, err := crbasic.Parse(raw)
			if err != nil {
				thisLogger.Error(err, "failed to parse file", "path", path)
				return nil
			}
			thisLogger.V(4).Info("Parsed file", "path", path, "tables", len(cfg))

			tableDefinitions[stationId][year] = append(tableDefinitions[stationId][year], cfg...)
		}

		if strings.ToLower(filepath.Ext(path)) == ".dat" {
			stationId, _, err := parseName(path)
			if err != nil {
				thisLogger.V(4).Error(err, "failed to parse name")
				return nil
			}
			if _, ok := dataFiles[stationId]; !ok {
				dataFiles[stationId] = []string{}
			}
			dataFiles[stationId] = append(dataFiles[stationId], filepath.Join(opts.DataDir, path))
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to walk data dir: %w", err)
	}

	group, _ := errgroup.WithContext(ctx)
	group.SetLimit(2 * runtime.NumCPU())
	for stationId, files := range dataFiles {
		group.Go(func() error {
			stationLogger := logger.WithValues("station", stationId)
			if err := data.ParseAll(stationLogger, files, tableDefinitions[stationId]); err != nil {
				stationLogger.Error(err, "failed to parse data")
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return fmt.Errorf("failed to wait for data files to be processed: %w", err)
	}

	return nil
}
