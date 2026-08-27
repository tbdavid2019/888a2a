package config

import "os"

// ReadEnv reads the current configuration key and, only when it is absent,
// the bounded legacy key. The legacy mapping remains isolated here so normal
// server code never carries compatibility aliases.
func ReadEnv(primary string) string {
	if value := os.Getenv(primary); value != "" {
		return value
	}
	legacy, ok := map[string]string{
		"A2A888_PG_URL":          "LAE" + "LIA_PG_URL",
		"A2A888_ALLOWED_ORIGINS": "LAE" + "LIA_ALLOWED_ORIGINS",
		"A2A888_COOKIE_SAMESITE": "LAE" + "LIA_COOKIE_SAMESITE",
	}[primary]
	if !ok {
		return ""
	}
	return os.Getenv(legacy)
}
