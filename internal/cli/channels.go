package cli

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
	Run: func(cmd *cobra.Command, args []string) {
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			fmt.Printf("Error parsing config: %v\n", err)
			os.Exit(1)
		}

		// Apply defaults if not set (viper unmarshal might not fill missing fields with struct defaults unless verified)
		// For simplicity, we assume viper has defaults or we handle empty strings.
		// Actually, let's manually check or rely on viper defaults set in root.

		client := tunarr.NewClient(cfg.Tunarr)
		channels, err := client.GetChannels()
		if err != nil {
			fmt.Printf("Error fetching channels: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tNumber\tName\tEnabled")
		for _, ch := range channels {
			fmt.Fprintf(w, "%s\t%d\t%s\t%v\n", ch.ID, ch.Number, ch.Name, ch.Enabled)
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(channelsCmd)
}
