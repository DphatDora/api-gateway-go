package config

// ServiceTarget represents a downstream service that the gateway proxies to.
type ServiceTarget struct {
	Name       string `mapstructure:"name"`
	Prefix     string `mapstructure:"prefix"`
	Target     string `mapstructure:"target"`
	HealthPath string `mapstructure:"healthPath"`
}
