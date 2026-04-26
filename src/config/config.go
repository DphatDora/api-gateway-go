package config

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

var (
	once   sync.Once
	config Config
)

type Config struct {
	App       App             `mapstructure:"app"`
	Log       Log             `mapstructure:"log"`
	Services  []ServiceTarget `mapstructure:"services"`
	Redis     Redis           `mapstructure:"redis"`
	RateLimit RateLimit       `mapstructure:"rateLimit"`
	Cache     Cache           `mapstructure:"cache"`
}

func LoadConfig() {
	once.Do(func() {
		// load .env config
		_ = godotenv.Load()

		// load yaml config
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("./config")

		if err := viper.ReadInConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "Config file error: %v\n", err)
			os.Exit(1)
		}

		// bind system environment variables
		viper.AutomaticEnv()
		viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

		bindEnvs()

		// load into struct
		if err := viper.Unmarshal(&config); err != nil {
			fmt.Fprintf(os.Stderr, "Config unmarshal error: %v\n", err)
			os.Exit(1)
		}
	})
}

func GetConfig() Config {
	LoadConfig()
	return config
}

func bindEnvs() {
	// App
	_ = viper.BindEnv("app.name", "APP_NAME")
	_ = viper.BindEnv("app.host", "HOST")
	_ = viper.BindEnv("app.port", "PORT")

	// Log
	_ = viper.BindEnv("log.level", "LOG_LEVEL")
	_ = viper.BindEnv("log.filePath", "LOG_FILE_PATH")
	_ = viper.BindEnv("log.maxSizeMB", "LOG_MAX_SIZE_MB")
	_ = viper.BindEnv("log.console", "LOG_CONSOLE")
	_ = viper.BindEnv("log.dashboardToken", "LOG_DASHBOARD_TOKEN")
	_ = viper.BindEnv("log.requestLogPath", "LOG_REQUEST_LOG_PATH")

	// Redis
	_ = viper.BindEnv("redis.host", "REDIS_HOST")
	_ = viper.BindEnv("redis.port", "REDIS_PORT")
	_ = viper.BindEnv("redis.password", "REDIS_PASSWORD")
	_ = viper.BindEnv("redis.db", "REDIS_DB")
	_ = viper.BindEnv("redis.poolSize", "REDIS_POOL_SIZE")
	_ = viper.BindEnv("redis.required", "REDIS_REQUIRED")
}
