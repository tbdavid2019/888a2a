package cmd

import (
	"strings"

	"github.com/tbdavid2019/888a2a/backend/common"
	"github.com/tbdavid2019/888a2a/backend/manager/config"
)

func getBaseProfile(_ string) *config.Profile {
	cfg := &config.Profile{
		Mode:           common.ReleaseModeProd,
		Port:           flags.port,
		PgURL:          config.ReadEnv("A2A888_PG_URL"),
		TLSCertDir:     flags.tlsCertDir,
		TLSDomain:      flags.tlsDomain,
		TrustProxy:     flags.trustProxy,
		PprofAddr:      flags.pprofAddr,
		AllowedOrigins: splitCSV(config.ReadEnv("A2A888_ALLOWED_ORIGINS")),
		CookieSameSite: config.ReadEnv("A2A888_COOKIE_SAMESITE"),
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
