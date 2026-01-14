package jellyfin

// Config holds the configuration for connecting to a Jellyfin instance.
type Config struct {
	URL        string `mapstructure:"url" yaml:"url" json:"url"`
	APIKey     string `mapstructure:"api_key" yaml:"api_key" json:"api_key"`
	UserID     string `mapstructure:"user_id" yaml:"user_id" json:"user_id,omitempty"`
	SyncLiveTV bool   `mapstructure:"sync_live_tv" yaml:"sync_live_tv" json:"sync_live_tv,omitempty"`
}
