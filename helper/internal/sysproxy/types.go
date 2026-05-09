package sysproxy

// Status represents the current system proxy configuration.
type Status struct {
	HTTPEnabled  bool   `json:"httpEnabled"`
	HTTPHost     string `json:"httpHost"`
	HTTPPort     int    `json:"httpPort"`
	HTTPSEnabled bool   `json:"httpsEnabled"`
	HTTPSHost    string `json:"httpsHost"`
	HTTPSPort    int    `json:"httpsPort"`
	SOCKSEnabled bool   `json:"socksEnabled"`
	SOCKSHost    string `json:"socksHost"`
	SOCKSPort    int    `json:"socksPort"`
}
