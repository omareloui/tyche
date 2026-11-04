package models

import (
	"slices"
	"strings"
)

// OnboardingStep is a step of the onboarding at /welcome, in order.
type OnboardingStep string

const (
	OnboardingRegion   OnboardingStep = "region"
	OnboardingAccounts OnboardingStep = "accounts"
	OnboardingSecurity OnboardingStep = "security"
	// OnboardingBadge offers the installed app's badge (or installing it).
	OnboardingBadge OnboardingStep = "badge"
	OnboardingDone  OnboardingStep = "done"
)

var OnboardingSteps = []OnboardingStep{OnboardingRegion, OnboardingAccounts, OnboardingSecurity, OnboardingBadge, OnboardingDone}

// OnboardingStepsOf are the steps shown: without the badge one when it isn't
// offered (not in the installed app, or no pushes).
func OnboardingStepsOf(badge bool) []OnboardingStep {
	if badge {
		return OnboardingSteps
	}
	return slices.DeleteFunc(slices.Clone(OnboardingSteps), func(s OnboardingStep) bool { return s == OnboardingBadge })
}

// ParseOnboardingStep is the step named s, the first one if none is.
func ParseOnboardingStep(s string) OnboardingStep {
	for _, step := range OnboardingSteps {
		if string(step) == s {
			return step
		}
	}
	return OnboardingRegion
}

// Index is the step's 0-based position.
func (s OnboardingStep) Index() int {
	for i, step := range OnboardingSteps {
		if step == s {
			return i
		}
	}
	return 0
}

// Next is the step after s, s itself for the last one.
func (s OnboardingStep) Next() OnboardingStep {
	return OnboardingSteps[min(s.Index()+1, len(OnboardingSteps)-1)]
}

// Prev is the step before s, s itself for the first one.
func (s OnboardingStep) Prev() OnboardingStep {
	return OnboardingSteps[max(s.Index()-1, 0)]
}

// timezoneCurrencies are the currencies of the countries of common time
// zones.
var timezoneCurrencies = map[string]string{
	"Africa/Cairo": "EGP", "Africa/Casablanca": "MAD", "Africa/Johannesburg": "ZAR", "Africa/Lagos": "NGN",
	"Africa/Nairobi": "KES", "Africa/Algiers": "DZD", "Africa/Tunis": "TND", "Africa/Khartoum": "SDG",
	"America/Chicago": "USD", "America/Los_Angeles": "USD", "America/New_York": "USD", "America/Denver": "USD",
	"America/Phoenix": "USD", "America/Anchorage": "USD", "America/Sao_Paulo": "BRL", "America/Toronto": "CAD",
	"America/Vancouver": "CAD", "America/Mexico_City": "MXN", "America/Argentina/Buenos_Aires": "ARS",
	"Asia/Amman": "JOD", "Asia/Baghdad": "IQD", "Asia/Beirut": "LBP", "Asia/Dubai": "AED", "Asia/Istanbul": "TRY",
	"Europe/Istanbul": "TRY", "Asia/Karachi": "PKR", "Asia/Kolkata": "INR", "Asia/Calcutta": "INR", "Asia/Kuwait": "KWD",
	"Asia/Qatar": "QAR", "Asia/Riyadh": "SAR", "Asia/Bahrain": "BHD", "Asia/Muscat": "OMR", "Asia/Shanghai": "CNY",
	"Asia/Singapore": "SGD", "Asia/Tokyo": "JPY", "Asia/Seoul": "KRW", "Asia/Hong_Kong": "HKD", "Asia/Jakarta": "IDR",
	"Asia/Manila": "PHP", "Asia/Bangkok": "THB", "Asia/Kuala_Lumpur": "MYR", "Asia/Dhaka": "BDT",
	"Australia/Sydney": "AUD", "Australia/Melbourne": "AUD", "Pacific/Auckland": "NZD",
	"Europe/London": "GBP", "Europe/Zurich": "CHF", "Europe/Stockholm": "SEK", "Europe/Oslo": "NOK",
	"Europe/Copenhagen": "DKK", "Europe/Warsaw": "PLN", "Europe/Prague": "CZK", "Europe/Budapest": "HUF",
	"Europe/Moscow": "RUB", "Europe/Kyiv": "UAH", "Europe/Kiev": "UAH", "Europe/Berlin": "EUR", "Europe/Madrid": "EUR",
	"Europe/Paris": "EUR", "Europe/Rome": "EUR", "Europe/Amsterdam": "EUR", "Europe/Brussels": "EUR",
	"Europe/Vienna": "EUR", "Europe/Dublin": "EUR", "Europe/Lisbon": "EUR", "Europe/Athens": "EUR",
	"Europe/Helsinki": "EUR",
}

// regionCurrencies are the currencies of the regions of browser languages
// (en-GB, ar-EG, ...).
var regionCurrencies = map[string]string{
	"US": "USD", "EG": "EGP", "AE": "AED", "SA": "SAR", "KW": "KWD", "QA": "QAR", "BH": "BHD", "OM": "OMR",
	"JO": "JOD", "LB": "LBP", "IQ": "IQD", "MA": "MAD", "DZ": "DZD", "TN": "TND", "SD": "SDG", "TR": "TRY",
	"GB": "GBP", "CA": "CAD", "AU": "AUD", "NZ": "NZD", "IN": "INR", "PK": "PKR", "BD": "BDT", "CN": "CNY",
	"JP": "JPY", "KR": "KRW", "SG": "SGD", "HK": "HKD", "ID": "IDR", "PH": "PHP", "TH": "THB", "MY": "MYR",
	"BR": "BRL", "MX": "MXN", "AR": "ARS", "ZA": "ZAR", "NG": "NGN", "KE": "KES", "CH": "CHF", "SE": "SEK",
	"NO": "NOK", "DK": "DKK", "PL": "PLN", "CZ": "CZK", "HU": "HUF", "RU": "RUB", "UA": "UAH",
	"DE": "EUR", "FR": "EUR", "ES": "EUR", "IT": "EUR", "NL": "EUR", "BE": "EUR", "AT": "EUR", "IE": "EUR",
	"PT": "EUR", "GR": "EUR", "FI": "EUR",
}

// SuggestCurrency guesses the code of the user's currency from their browser's
// time zone, else the regions of its languages (en-US is often the default
// whatever the country, so the time zone goes first). "" when unknown.
func SuggestCurrency(timezone string, langs []string) string {
	if c, ok := timezoneCurrencies[strings.TrimSpace(timezone)]; ok {
		return c
	}
	for _, l := range langs {
		parts := strings.FieldsFunc(l, func(r rune) bool { return r == '-' || r == '_' })
		for _, p := range parts[min(1, len(parts)):] {
			if c, ok := regionCurrencies[strings.ToUpper(p)]; ok {
				return c
			}
		}
	}
	return ""
}
