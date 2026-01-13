package sonarr

// Config holds the configuration for connecting to a Sonarr instance.
type Config struct {
	URL    string `mapstructure:"url" yaml:"url" json:"url"`
	APIKey string `mapstructure:"api_key" yaml:"api_key" json:"api_key"`
}
