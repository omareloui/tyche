// Package countries lists the countries (ISO 3166 codes, name, currency) and
// tells the country of an IANA time zone (the tz database's zone.tab).
package countries

import (
	"bufio"
	_ "embed"
	"slices"
	"strings"
	"sync"
)

// Country is a country with its ISO 3166 codes and main currency (ISO 4217).
type Country struct {
	ISO2     string
	ISO3     string
	Name     string
	Currency string
}

// all are the countries, by name. Territories without their own data in the
// price statistics are left out.
var all = []Country{
	{"AF", "AFG", "Afghanistan", "AFN"},
	{"AL", "ALB", "Albania", "ALL"},
	{"DZ", "DZA", "Algeria", "DZD"},
	{"AD", "AND", "Andorra", "EUR"},
	{"AO", "AGO", "Angola", "AOA"},
	{"AG", "ATG", "Antigua and Barbuda", "XCD"},
	{"AR", "ARG", "Argentina", "ARS"},
	{"AM", "ARM", "Armenia", "AMD"},
	{"AW", "ABW", "Aruba", "AWG"},
	{"AU", "AUS", "Australia", "AUD"},
	{"AT", "AUT", "Austria", "EUR"},
	{"AZ", "AZE", "Azerbaijan", "AZN"},
	{"BS", "BHS", "Bahamas", "BSD"},
	{"BH", "BHR", "Bahrain", "BHD"},
	{"BD", "BGD", "Bangladesh", "BDT"},
	{"BB", "BRB", "Barbados", "BBD"},
	{"BY", "BLR", "Belarus", "BYN"},
	{"BE", "BEL", "Belgium", "EUR"},
	{"BZ", "BLZ", "Belize", "BZD"},
	{"BJ", "BEN", "Benin", "XOF"},
	{"BM", "BMU", "Bermuda", "BMD"},
	{"BT", "BTN", "Bhutan", "BTN"},
	{"BO", "BOL", "Bolivia", "BOB"},
	{"BA", "BIH", "Bosnia and Herzegovina", "BAM"},
	{"BW", "BWA", "Botswana", "BWP"},
	{"BR", "BRA", "Brazil", "BRL"},
	{"BN", "BRN", "Brunei", "BND"},
	{"BG", "BGR", "Bulgaria", "BGN"},
	{"BF", "BFA", "Burkina Faso", "XOF"},
	{"BI", "BDI", "Burundi", "BIF"},
	{"CV", "CPV", "Cabo Verde", "CVE"},
	{"KH", "KHM", "Cambodia", "KHR"},
	{"CM", "CMR", "Cameroon", "XAF"},
	{"CA", "CAN", "Canada", "CAD"},
	{"KY", "CYM", "Cayman Islands", "KYD"},
	{"CF", "CAF", "Central African Republic", "XAF"},
	{"TD", "TCD", "Chad", "XAF"},
	{"CL", "CHL", "Chile", "CLP"},
	{"CN", "CHN", "China", "CNY"},
	{"CO", "COL", "Colombia", "COP"},
	{"KM", "COM", "Comoros", "KMF"},
	{"CD", "COD", "Congo (DR)", "CDF"},
	{"CG", "COG", "Congo", "XAF"},
	{"CR", "CRI", "Costa Rica", "CRC"},
	{"CI", "CIV", "Côte d'Ivoire", "XOF"},
	{"HR", "HRV", "Croatia", "EUR"},
	{"CU", "CUB", "Cuba", "CUP"},
	{"CW", "CUW", "Curaçao", "ANG"},
	{"CY", "CYP", "Cyprus", "EUR"},
	{"CZ", "CZE", "Czechia", "CZK"},
	{"DK", "DNK", "Denmark", "DKK"},
	{"DJ", "DJI", "Djibouti", "DJF"},
	{"DM", "DMA", "Dominica", "XCD"},
	{"DO", "DOM", "Dominican Republic", "DOP"},
	{"EC", "ECU", "Ecuador", "USD"},
	{"EG", "EGY", "Egypt", "EGP"},
	{"SV", "SLV", "El Salvador", "USD"},
	{"GQ", "GNQ", "Equatorial Guinea", "XAF"},
	{"ER", "ERI", "Eritrea", "ERN"},
	{"EE", "EST", "Estonia", "EUR"},
	{"SZ", "SWZ", "Eswatini", "SZL"},
	{"ET", "ETH", "Ethiopia", "ETB"},
	{"FJ", "FJI", "Fiji", "FJD"},
	{"FI", "FIN", "Finland", "EUR"},
	{"FR", "FRA", "France", "EUR"},
	{"GA", "GAB", "Gabon", "XAF"},
	{"GM", "GMB", "Gambia", "GMD"},
	{"GE", "GEO", "Georgia", "GEL"},
	{"DE", "DEU", "Germany", "EUR"},
	{"GH", "GHA", "Ghana", "GHS"},
	{"GR", "GRC", "Greece", "EUR"},
	{"GD", "GRD", "Grenada", "XCD"},
	{"GT", "GTM", "Guatemala", "GTQ"},
	{"GN", "GIN", "Guinea", "GNF"},
	{"GW", "GNB", "Guinea-Bissau", "XOF"},
	{"GY", "GUY", "Guyana", "GYD"},
	{"HT", "HTI", "Haiti", "HTG"},
	{"HN", "HND", "Honduras", "HNL"},
	{"HK", "HKG", "Hong Kong", "HKD"},
	{"HU", "HUN", "Hungary", "HUF"},
	{"IS", "ISL", "Iceland", "ISK"},
	{"IN", "IND", "India", "INR"},
	{"ID", "IDN", "Indonesia", "IDR"},
	{"IR", "IRN", "Iran", "IRR"},
	{"IQ", "IRQ", "Iraq", "IQD"},
	{"IE", "IRL", "Ireland", "EUR"},
	{"IL", "ISR", "Israel", "ILS"},
	{"IT", "ITA", "Italy", "EUR"},
	{"JM", "JAM", "Jamaica", "JMD"},
	{"JP", "JPN", "Japan", "JPY"},
	{"JO", "JOR", "Jordan", "JOD"},
	{"KZ", "KAZ", "Kazakhstan", "KZT"},
	{"KE", "KEN", "Kenya", "KES"},
	{"KI", "KIR", "Kiribati", "AUD"},
	{"KR", "KOR", "South Korea", "KRW"},
	{"XK", "XKX", "Kosovo", "EUR"},
	{"KW", "KWT", "Kuwait", "KWD"},
	{"KG", "KGZ", "Kyrgyzstan", "KGS"},
	{"LA", "LAO", "Laos", "LAK"},
	{"LV", "LVA", "Latvia", "EUR"},
	{"LB", "LBN", "Lebanon", "LBP"},
	{"LS", "LSO", "Lesotho", "LSL"},
	{"LR", "LBR", "Liberia", "LRD"},
	{"LY", "LBY", "Libya", "LYD"},
	{"LI", "LIE", "Liechtenstein", "CHF"},
	{"LT", "LTU", "Lithuania", "EUR"},
	{"LU", "LUX", "Luxembourg", "EUR"},
	{"MO", "MAC", "Macao", "MOP"},
	{"MG", "MDG", "Madagascar", "MGA"},
	{"MW", "MWI", "Malawi", "MWK"},
	{"MY", "MYS", "Malaysia", "MYR"},
	{"MV", "MDV", "Maldives", "MVR"},
	{"ML", "MLI", "Mali", "XOF"},
	{"MT", "MLT", "Malta", "EUR"},
	{"MH", "MHL", "Marshall Islands", "USD"},
	{"MR", "MRT", "Mauritania", "MRU"},
	{"MU", "MUS", "Mauritius", "MUR"},
	{"MX", "MEX", "Mexico", "MXN"},
	{"FM", "FSM", "Micronesia", "USD"},
	{"MD", "MDA", "Moldova", "MDL"},
	{"MC", "MCO", "Monaco", "EUR"},
	{"MN", "MNG", "Mongolia", "MNT"},
	{"ME", "MNE", "Montenegro", "EUR"},
	{"MA", "MAR", "Morocco", "MAD"},
	{"MZ", "MOZ", "Mozambique", "MZN"},
	{"MM", "MMR", "Myanmar", "MMK"},
	{"NA", "NAM", "Namibia", "NAD"},
	{"NR", "NRU", "Nauru", "AUD"},
	{"NP", "NPL", "Nepal", "NPR"},
	{"NL", "NLD", "Netherlands", "EUR"},
	{"NZ", "NZL", "New Zealand", "NZD"},
	{"NI", "NIC", "Nicaragua", "NIO"},
	{"NE", "NER", "Niger", "XOF"},
	{"NG", "NGA", "Nigeria", "NGN"},
	{"MK", "MKD", "North Macedonia", "MKD"},
	{"NO", "NOR", "Norway", "NOK"},
	{"OM", "OMN", "Oman", "OMR"},
	{"PK", "PAK", "Pakistan", "PKR"},
	{"PW", "PLW", "Palau", "USD"},
	{"PS", "PSE", "Palestine", "ILS"},
	{"PA", "PAN", "Panama", "PAB"},
	{"PG", "PNG", "Papua New Guinea", "PGK"},
	{"PY", "PRY", "Paraguay", "PYG"},
	{"PE", "PER", "Peru", "PEN"},
	{"PH", "PHL", "Philippines", "PHP"},
	{"PL", "POL", "Poland", "PLN"},
	{"PT", "PRT", "Portugal", "EUR"},
	{"PR", "PRI", "Puerto Rico", "USD"},
	{"QA", "QAT", "Qatar", "QAR"},
	{"RO", "ROU", "Romania", "RON"},
	{"RU", "RUS", "Russia", "RUB"},
	{"RW", "RWA", "Rwanda", "RWF"},
	{"KN", "KNA", "Saint Kitts and Nevis", "XCD"},
	{"LC", "LCA", "Saint Lucia", "XCD"},
	{"VC", "VCT", "Saint Vincent and the Grenadines", "XCD"},
	{"WS", "WSM", "Samoa", "WST"},
	{"SM", "SMR", "San Marino", "EUR"},
	{"ST", "STP", "São Tomé and Príncipe", "STN"},
	{"SA", "SAU", "Saudi Arabia", "SAR"},
	{"SN", "SEN", "Senegal", "XOF"},
	{"RS", "SRB", "Serbia", "RSD"},
	{"SC", "SYC", "Seychelles", "SCR"},
	{"SL", "SLE", "Sierra Leone", "SLE"},
	{"SG", "SGP", "Singapore", "SGD"},
	{"SX", "SXM", "Sint Maarten", "ANG"},
	{"SK", "SVK", "Slovakia", "EUR"},
	{"SI", "SVN", "Slovenia", "EUR"},
	{"SB", "SLB", "Solomon Islands", "SBD"},
	{"SO", "SOM", "Somalia", "SOS"},
	{"ZA", "ZAF", "South Africa", "ZAR"},
	{"SS", "SSD", "South Sudan", "SSP"},
	{"ES", "ESP", "Spain", "EUR"},
	{"LK", "LKA", "Sri Lanka", "LKR"},
	{"SD", "SDN", "Sudan", "SDG"},
	{"SR", "SUR", "Suriname", "SRD"},
	{"SE", "SWE", "Sweden", "SEK"},
	{"CH", "CHE", "Switzerland", "CHF"},
	{"SY", "SYR", "Syria", "SYP"},
	{"TW", "TWN", "Taiwan", "TWD"},
	{"TJ", "TJK", "Tajikistan", "TJS"},
	{"TZ", "TZA", "Tanzania", "TZS"},
	{"TH", "THA", "Thailand", "THB"},
	{"TL", "TLS", "Timor-Leste", "USD"},
	{"TG", "TGO", "Togo", "XOF"},
	{"TO", "TON", "Tonga", "TOP"},
	{"TT", "TTO", "Trinidad and Tobago", "TTD"},
	{"TN", "TUN", "Tunisia", "TND"},
	{"TR", "TUR", "Türkiye", "TRY"},
	{"TM", "TKM", "Turkmenistan", "TMT"},
	{"TC", "TCA", "Turks and Caicos Islands", "USD"},
	{"TV", "TUV", "Tuvalu", "AUD"},
	{"UG", "UGA", "Uganda", "UGX"},
	{"UA", "UKR", "Ukraine", "UAH"},
	{"AE", "ARE", "United Arab Emirates", "AED"},
	{"GB", "GBR", "United Kingdom", "GBP"},
	{"US", "USA", "United States", "USD"},
	{"UY", "URY", "Uruguay", "UYU"},
	{"UZ", "UZB", "Uzbekistan", "UZS"},
	{"VU", "VUT", "Vanuatu", "VUV"},
	{"VE", "VEN", "Venezuela", "VES"},
	{"VN", "VNM", "Vietnam", "VND"},
	{"YE", "YEM", "Yemen", "YER"},
	{"ZM", "ZMB", "Zambia", "ZMW"},
	{"ZW", "ZWE", "Zimbabwe", "ZWG"},
}

// All are the countries, by name.
func All() []Country {
	return slices.Clone(all)
}

var (
	byISO2 = map[string]Country{}
	byISO3 = map[string]Country{}
)

func init() {
	for _, c := range all {
		byISO2[c.ISO2] = c
		byISO3[c.ISO3] = c
	}
}

// ByISO2 is the country of a 2 letter code (any case).
func ByISO2(code string) (Country, bool) {
	c, ok := byISO2[strings.ToUpper(strings.TrimSpace(code))]
	return c, ok
}

// ByISO3 is the country of a 3 letter code (any case).
func ByISO3(code string) (Country, bool) {
	c, ok := byISO3[strings.ToUpper(strings.TrimSpace(code))]
	return c, ok
}

//go:embed zone.tab
var zoneTab string

// zoneAliases are older zone names still sent by browsers, not in zone.tab.
var zoneAliases = map[string]string{
	"Asia/Calcutta":                    "IN",
	"Asia/Saigon":                      "VN",
	"Asia/Katmandu":                    "NP",
	"Asia/Rangoon":                     "MM",
	"Asia/Istanbul":                    "TR",
	"Europe/Kiev":                      "UA",
	"Europe/Istanbul":                  "TR",
	"America/Buenos_Aires":             "AR",
	"America/Argentina/ComodRivadavia": "AR",
	"Africa/Asmera":                    "ER",
	"Atlantic/Faeroe":                  "FO",
	"Pacific/Truk":                     "FM",
	"Pacific/Ponape":                   "FM",
	"Egypt":                            "EG",
	"Turkey":                           "TR",
	"Japan":                            "JP",
	"Singapore":                        "SG",
	"Israel":                           "IL",
	"Iran":                             "IR",
	"GB":                               "GB",
	"PRC":                              "CN",
	"ROC":                              "TW",
	"ROK":                              "KR",
	"Hongkong":                         "HK",
	"Portugal":                         "PT",
	"Poland":                           "PL",
	"Eire":                             "IE",
	"Europe/Belfast":                   "GB",
	"US/Eastern":                       "US",
	"US/Central":                       "US",
	"US/Mountain":                      "US",
	"US/Pacific":                       "US",
	"Canada/Eastern":                   "CA",
	"Australia/ACT":                    "AU",
	"Australia/NSW":                    "AU",
}

var (
	zonesOnce sync.Once
	zones     map[string]string
)

func loadZones() {
	zones = make(map[string]string, 450)
	sc := bufio.NewScanner(strings.NewReader(zoneTab))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		code, zone, ok := strings.Cut(line, "\t")
		if ok {
			zones[zone] = code
		}
	}
	for zone, code := range zoneAliases {
		if _, ok := zones[zone]; !ok {
			zones[zone] = code
		}
	}
}

// FromTimezone is the country of an IANA time zone, false when it's not a
// country's (UTC) or the country isn't listed.
func FromTimezone(tz string) (Country, bool) {
	zonesOnce.Do(loadZones)
	code, ok := zones[strings.TrimSpace(tz)]
	if !ok {
		return Country{}, false
	}
	return ByISO2(code)
}
