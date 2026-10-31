package config

import (
	"fmt"
	"strings"
	"time"
)

func GetApplicationPort() int {
	return getEnvironmentInt("PORT")
}

func GetAppEnv() string {
	return getEnvironmentValueFromOptions("APP_ENV", []string{"development", "production"})
}

func GetLogLevel() int {
	return getEnvironmentIntWithDefault("LOG_LEVEL", 0)
}

func GetJWTSecret() []byte {
	return []byte(getEnvironmentValue("JWT_SECRET"))
}

func GetOAuthCBEndpoint() string {
	domain := getEnvironmentValue("OAUTH_CB_ORIGIN")
	return fmt.Sprintf("%s/oauth/cb", domain)
}

func GetGoogleOAuthClientID() string {
	return getEnvironmentValue("GOOGLE_CLIENT_ID")
}

func GetGoogleOAuthClientSecret() string {
	return getEnvironmentValue("GOOGLE_CLIENT_SECRET")
}

// GetFxRateFetchInterval is how often the fx rates job refetches the rates.
func GetFxRateFetchInterval() time.Duration {
	return time.Hour * time.Duration(max(getEnvironmentIntWithDefault("FX_RATE_FETCH_INTERVAL_HOURS", 6), 1))
}

func GetSharedDBPath() string {
	return getEnvironmentValueWithDefault("SHARED_DB_PATH", "./data/shared.sqlite")
}

// GetAdminTenantIDs are the tenant IDs (`{provider_prefix}_{oauth_id}`, as
// the database file names) allowed on the admin pages.
func GetAdminTenantIDs() []string {
	var ids []string
	for id := range strings.SplitSeq(getEnvironmentValueWithDefault("ADMIN_TENANT_IDS", ""), ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func GetExtraBrandIconsDir() string {
	return getEnvironmentValueWithDefault("EXTRA_BRAND_ICONS_DIR", "./web/brand-icons")
}

func GetSimpleIconsDir() string {
	return getEnvironmentValueWithDefault("SIMPLE_ICONS_DIR", "./third_party/simple-icons")
}
