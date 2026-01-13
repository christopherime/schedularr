package radarr

// Config holds the configuration for connecting to a Radarr instance.
type Config struct {
	URL                string `mapstructure:"url" yaml:"url" json:"url"`
	APIKey             string `mapstructure:"api_key" yaml:"api_key" json:"api_key"`
	ExcludeMissingFile bool   `mapstructure:"exclude_missing_file" yaml:"exclude_missing_file" json:"exclude_missing_file,omitempty"`
}
