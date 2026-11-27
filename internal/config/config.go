package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/omareloui/tyche/internal/gemini"
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

// GetColFetchInterval is how often the cost of living job refetches the
// price data (they change a few times a year).
func GetColFetchInterval() time.Duration {
	return 24 * time.Hour * time.Duration(max(getEnvironmentIntWithDefault("COL_FETCH_INTERVAL_DAYS", 30), 1))
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
	// SearchPerK is dollars per thousand Google Search queries of the
	// grounded calls (the city estimates), the free tier left out.
	SearchPerK    float64
	ScansPerMonth int
	// SMSGuessesPerMonth is how many sms templates a tenant may have
	// guessed a calendar month.
	SMSGuessesPerMonth int
	// ColEstimatesPerMonth is how many city estimates (cost of living) a
	// tenant may ask for a calendar month.
	ColEstimatesPerMonth int
	// TaxRulesPerMonth is how many tax rules lookups (salary calculator) a
	// tenant may ask for a calendar month.
	TaxRulesPerMonth int
	// VisaRulesPerMonth is how many visa requirements lookups (visa
	// readiness) a tenant may ask for a calendar month.
	VisaRulesPerMonth int
	// MerchantModel matches the sms' unknown merchants (a call per ingested
	// batch with some), Model when empty: a cheap model fits.
	MerchantModel string
	// MerchantThinkingLevel is MerchantModel's, ThinkingLevel when empty
	// (the models don't all take MINIMAL).
	MerchantThinkingLevel string
	// MerchantGuessesPerMonth is how many merchant matching calls a tenant
	// may make a calendar month.
	MerchantGuessesPerMonth int
	// QuickModel reads quick add's texts (a call per text), Model when empty.
	QuickModel string
	// QuickAddsPerMonth is how many quick add texts a tenant may have read
	// a calendar month.
	QuickAddsPerMonth int
	// SplitGuessesPerMonth is how many split bills a tenant may have read
	// from a sentence a calendar month.
	SplitGuessesPerMonth int
	// AskQuestionsPerMonth is how many questions a tenant may have turned
	// into SQL a calendar month (the console's Ask tab).
	AskQuestionsPerMonth int
	// StatementImportsPerMonth is how many PDF statements a tenant may have
	// read into an import a calendar month.
	StatementImportsPerMonth int
}

// Prices are the calls' prices.
func (c ReceiptOCR) Prices() gemini.Prices {
	return gemini.Prices{InputPerM: c.InputPerM, OutputPerM: c.OutputPerM, SearchPerK: c.SearchPerK}
}

// GetReceiptOCR reads the GEMINI_*, RECEIPT_SCANS_PER_MONTH,
// SMS_GUESSES_PER_MONTH, COL_ESTIMATES_PER_MONTH, TAX_RULES_PER_MONTH,
// VISA_RULES_PER_MONTH, MERCHANT_GUESSES_PER_MONTH, QUICK_ADDS_PER_MONTH,
// SPLIT_GUESSES_PER_MONTH, ASK_QUESTIONS_PER_MONTH and
// STATEMENT_IMPORTS_PER_MONTH variables.
func GetReceiptOCR() ReceiptOCR {
	return ReceiptOCR{
		APIKey:                   getEnvironmentValueWithDefault("GEMINI_API_KEY", ""),
		Model:                    getEnvironmentValueWithDefault("GEMINI_MODEL", "gemini-3.8-flash"),
		ThinkingLevel:            getEnvironmentValueWithDefault("GEMINI_THINKING_LEVEL", "LOW"),
		InputPerM:                getEnvironmentFloatWithDefault("GEMINI_PRICE_INPUT_PER_M", 0.75),
		OutputPerM:               getEnvironmentFloatWithDefault("GEMINI_PRICE_OUTPUT_PER_M", 3.75),
		SearchPerK:               getEnvironmentFloatWithDefault("GEMINI_PRICE_SEARCH_PER_K", 14),
		ScansPerMonth:            max(getEnvironmentIntWithDefault("RECEIPT_SCANS_PER_MONTH", 50), 0),
		SMSGuessesPerMonth:       max(getEnvironmentIntWithDefault("SMS_GUESSES_PER_MONTH", 30), 0),
		ColEstimatesPerMonth:     max(getEnvironmentIntWithDefault("COL_ESTIMATES_PER_MONTH", 20), 0),
		TaxRulesPerMonth:         max(getEnvironmentIntWithDefault("TAX_RULES_PER_MONTH", 10), 0),
		VisaRulesPerMonth:        max(getEnvironmentIntWithDefault("VISA_RULES_PER_MONTH", 10), 0),
		MerchantModel:            getEnvironmentValueWithDefault("GEMINI_MERCHANT_MODEL", ""),
		MerchantThinkingLevel:    getEnvironmentValueWithDefault("GEMINI_MERCHANT_THINKING_LEVEL", ""),
		MerchantGuessesPerMonth:  max(getEnvironmentIntWithDefault("MERCHANT_GUESSES_PER_MONTH", 100), 0),
		QuickModel:               getEnvironmentValueWithDefault("GEMINI_QUICK_MODEL", ""),
		QuickAddsPerMonth:        max(getEnvironmentIntWithDefault("QUICK_ADDS_PER_MONTH", 200), 0),
		SplitGuessesPerMonth:     max(getEnvironmentIntWithDefault("SPLIT_GUESSES_PER_MONTH", 30), 0),
		AskQuestionsPerMonth:     max(getEnvironmentIntWithDefault("ASK_QUESTIONS_PER_MONTH", 50), 0),
		StatementImportsPerMonth: max(getEnvironmentIntWithDefault("STATEMENT_IMPORTS_PER_MONTH", 10), 0),
	}
}
