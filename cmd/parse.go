package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NWACus/weather-telemetry/cmd/parse"
)

func NewParseCommand() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:           "parse",
		Short:         "Parse raw weather data.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	opts := parse.DefaultOptions()
	if err := parse.BindOptions(opts, cmd); err != nil {
		return nil, fmt.Errorf("failed to bind options: %w", err)
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		validated, err := opts.Validate()
		if err != nil {
			return err
		}
		completed, err := validated.Complete(cmd.Context())
		if err != nil {
			return err
		}
		return completed.Run(cmd.Context())
	}

	return cmd, nil
}
