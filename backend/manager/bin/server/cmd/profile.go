package cmd

import (
	"os"
	"strings"

	"github.com/tbdavid2019/888a2a/backend/common"
	"github.com/tbdavid2019/888a2a/backend/manager/config"
)

func getEnvWithFallback(primary, fallback string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	return os.Getenv(fallback)
}

func getBaseProfile(_ string) *config.Profile {
	legacyPrefix := "LAE" + "LIA_"
	cfg := &config.Profile{
		Mode:           common.ReleaseModeProd,
		Port:           flags.port,
		PgURL:          getEnvWithFallback("A2A888_PG_URL", legacyPrefix+"PG_URL"),
		TLSCertDir:     flags.tlsCertDir,
		TLSDomain:      flags.tlsDomain,
		TrustProxy:     flags.trustProxy,
		PprofAddr:      flags.pprofAddr,
		AllowedOrigins: splitCSV(getEnvWithFallback("A2A888_ALLOWED_ORIGINS", legacyPrefix+"ALLOWED_ORIGINS")),
		CookieSameSite: getEnvWithFallback("A2A888_COOKIE_SAMESITE", legacyPrefix+"COOKIE_SAMESITE"),
	}

	if flags.tlsHost != "" {
		cfg.TLSHosts = strings.Split(flags.tlsHost, ",")
	}

	cfg.RuntimeDebug.Store(flags.debug)
	return cfg
}

// splitCSV splits a comma-separated list, trimming whitespace and dropping
// empty entries.
func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
