package config

// Database holds connection configuration for Postgres, MongoDB, and Redis.
type Database struct {
	Postgres PostgresConfig `mapstructure:"postgres"`
	Mongo    MongoConfig    `mapstructure:"mongo"`
	Redis    DatabaseRedis  `mapstructure:"redis"`
}

// PostgresConfig holds the Postgres connection DSN.
type PostgresConfig struct {
	DSN string `mapstructure:"dsn"`
}

// MongoConfig holds the MongoDB connection URI.
type MongoConfig struct {
	URI string `mapstructure:"uri"`
}

// DatabaseRedis holds the Redis URL used for database status checks only.
// (Separate from the main Redis used for rate limiting/caching.)
type DatabaseRedis struct {
	URL string `mapstructure:"url"`
}
