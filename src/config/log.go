package config

type Log struct {
	Level              string `mapstructure:"level"`
	FilePath           string `mapstructure:"filePath"`
	MaxSizeMB          int    `mapstructure:"maxSizeMB"`
	Console            bool   `mapstructure:"console"`
	DashboardToken     string `mapstructure:"dashboardToken"`
	RequestLogPath     string `mapstructure:"requestLogPath"`
	RequestLogMaxSizeMB int   `mapstructure:"requestLogMaxSizeMB"`
}
