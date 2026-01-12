// Package cli provides command-line interface commands for Schedularr.
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/tunarr"
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
		channels, err := client.GetChannels()
		if err != nil {
			fmt.Printf("Error fetching channels: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		if _, err := fmt.Fprintln(w, "ID\tNumber\tName\tEnabled"); err != nil {
			fmt.Printf("Error writing header: %v\n", err)
			os.Exit(1)
		}
		for _, ch := range channels {
			fmt.Fprintf(w, "%s\t%d\t%s\t%v\n", ch.ID, ch.Number, ch.Name, ch.Enabled)
		}
		if err := w.Flush(); err != nil {
			fmt.Printf("Error flushing output: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(channelsCmd)
}
