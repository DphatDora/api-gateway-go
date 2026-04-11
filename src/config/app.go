package config

type App struct {
	Name         string   `mapstructure:"name"`
	Host         string   `mapstructure:"host"`
	Port         int      `mapstructure:"port"`
	Debug        bool     `mapstructure:"debug"`
	ReadTimeout  int      `mapstructure:"readTimeout"`
	WriteTimeout int      `mapstructure:"writeTimeout"`
	Whitelist    []string `mapstructure:"whitelist"`
}
