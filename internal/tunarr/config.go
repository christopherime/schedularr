package tunarr

// Config holds the configuration for connecting to a Tunarr instance.
type Config struct {
	URL    string `mapstructure:"url" yaml:"url"`
	APIKey string `mapstructure:"api_key" yaml:"api_key"`
}
