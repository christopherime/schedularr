// Package cmd provides command-line interface commands for Schedularr.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/external/tunarr"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var channelsCmd = &cobra.Command{
	Use:   "channels",
	Short: "List all Tunarr channels",
	Run: func(_ *cobra.Command, _ []string) {
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			fmt.Printf("Error parsing config: %v\n", err)
			os.Exit(1)
		}

		client := tunarr.NewClient(cfg.Tunarr)
		channels, err := client.GetChannels(context.Background())
		if err != nil {
			fmt.Printf("Error fetching channels: %v\n", err)
			os.Exit(1)
		}

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"ID", "Number", "Name", "Group"})
		for _, ch := range channels {
			t.AppendRow(table.Row{ch.ID, ch.Number, ch.Name, ch.GroupTitle})
		}
		t.SetStyle(table.StyleLight)
		t.Render()
	},
}

func init() {
	rootCmd.AddCommand(channelsCmd)
}
