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
	return fmt.Sprintf("%s/oauth/cb", GetOrigin())
}

// GetOrigin is the site's origin (OAUTH_CB_ORIGIN), e.g. for the passkeys.
func GetOrigin() string {
	return getEnvironmentValue("OAUTH_CB_ORIGIN")
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

// GetVAPIDKeys are the keys signing the Web Push notifications of the
// installed apps' badge (generate with `go run ./cmd/vapid`), the subject a
// mailto: or https: contact. Empty keys turn the badge's pushes off.
func GetVAPIDKeys() (public, private, subject string) {
	return getEnvironmentValueWithDefault("VAPID_PUBLIC_KEY", ""),
		getEnvironmentValueWithDefault("VAPID_PRIVATE_KEY", ""),
		getEnvironmentValueWithDefault("VAPID_SUBJECT", "")
}

// ReceiptOCR is the config of reading receipts with Gemini (an empty API key
// turns it off). Prices are dollars per million tokens, thinking ones billed
// as output.
type ReceiptOCR struct {
	APIKey        string
	Model         string
	ThinkingLevel string
	InputPerM     float64
	OutputPerM    float64
	ScansPerMonth int
}

// GetReceiptOCR reads the GEMINI_* and RECEIPT_SCANS_PER_MONTH variables.
func GetReceiptOCR() ReceiptOCR {
	return ReceiptOCR{
		APIKey:        getEnvironmentValueWithDefault("GEMINI_API_KEY", ""),
		Model:         getEnvironmentValueWithDefault("GEMINI_MODEL", "gemini-3.8-flash"),
		ThinkingLevel: getEnvironmentValueWithDefault("GEMINI_THINKING_LEVEL", "LOW"),
		InputPerM:     getEnvironmentFloatWithDefault("GEMINI_PRICE_INPUT_PER_M", 0.75),
		OutputPerM:    getEnvironmentFloatWithDefault("GEMINI_PRICE_OUTPUT_PER_M", 3.75),
		ScansPerMonth: max(getEnvironmentIntWithDefault("RECEIPT_SCANS_PER_MONTH", 50), 0),
	}
}
