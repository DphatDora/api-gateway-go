package config

// RateLimitGlobal is the default limit applied to every request.
type RateLimitGlobal struct {
	Limit  int `mapstructure:"limit"`
	Window int `mapstructure:"window"` // seconds
}

// RateLimitRoute overrides the global limit for requests whose path starts with Pattern.
type RateLimitRoute struct {
	Pattern string `mapstructure:"pattern"`
	Limit   int    `mapstructure:"limit"`
	Window  int    `mapstructure:"window"` // seconds
}

// RateLimit holds the full rate-limiting configuration.
type RateLimit struct {
	Enabled bool             `mapstructure:"enabled"`
	Global  RateLimitGlobal  `mapstructure:"global"`
	Routes  []RateLimitRoute `mapstructure:"routes"`
}

// CacheRoute pairs a path prefix with its cache TTL.
type CacheRoute struct {
	Pattern string `mapstructure:"pattern"`
	TTL     int    `mapstructure:"ttl"` // seconds
}

// Cache holds the response-caching configuration.
type Cache struct {
	Enabled bool         `mapstructure:"enabled"`
	Routes  []CacheRoute `mapstructure:"routes"`
}
