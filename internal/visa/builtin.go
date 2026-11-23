package visa

import (
	"math"
	"sort"
	"strings"

	"github.com/omareloui/tyche/internal/payroll"
)

// Reference amounts of 2026 the built in visas are worked out from, so a
// yearly update is one constant. Checked 2026-11 against the sources named.
const (
	builtinYear = 2026

	// esSMI is Spain's minimum wage a month in 14 pays (RD 126/2026,
	// https://www.boe.es/buscar/doc.php?id=BOE-A-2026-3815); the visas use it
	// spread over 12 months.
	esSMI = 1221.0
	// esIPREM is Spain's IPREM a month (unchanged since 2023).
	esIPREM = 600.0
	// ptRMMG is Portugal's minimum wage a month.
	ptRMMG = 920.0
	// deStudentMonth is a German student's blocked account a month (BAföG
	// rate), deChancenMonth the Opportunity Card's.
	deStudentMonth = 992.0
	deChancenMonth = 1091.0
	// deBlueCard / deBlueCardReduced are the EU Blue Card's gross salaries a
	// year (the reduced one: shortage occupations, recent graduates, IT
	// specialists).
	deBlueCard        = 50700.0
	deBlueCardReduced = 45934.20
	// eeAvgWage is Estonia's average gross wage a month the work permits use
	// (applications from 5 March 2026).
	eeAvgWage = 2092.0
	// nzMedianHour is New Zealand's median wage an hour (from 9 March 2026).
	nzMedianHour = 35.0
)

// esSMIMonthly is the SMI spread over 12 months.
var esSMIMonthly = round2(esSMI * 14 / 12)

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// caFunds is Canada's proof of settlement funds by family size (IRCC table
// of 2025-07-07), its last increment repeating for every member after.
var caFunds = []float64{15263, 19001, 23360, 28362, 32168, 36280, 40392}

func caFundsRequirement() Requirement {
	per := make([]float64, 0, len(caFunds))
	for i := 1; i < len(caFunds); i++ {
		per = append(per, caFunds[i]-caFunds[i-1])
	}
	return Requirement{
		Kind: KindSavings, Period: PeriodOnce, Amount: caFunds[0], PerDependent: per,
		Text: "Settlement funds by family size (spouse and dependent children counted even when not coming along); not borrowed, available when applying and landing.",
	}
}

func check(text string) Requirement { return Requirement{Kind: KindCheck, Text: text} }

// builtin is a visa's rules written here.
type builtin struct {
	key, name string
	category  Category
	options   []Option
	rule      func(opts map[string]string) Rule
}

// Visa names a visa a country offers.
type Visa struct {
	Key, Name string
	Category  Category
}

var builtins = map[string][]builtin{
	"ES": {
		{key: "digital-nomad", name: "Digital Nomad Visa", category: CategoryNomad, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindIncome, Period: PeriodMonth, Amount: round2(2 * esSMIMonthly), PerDependent: []float64{round2(0.75 * esSMIMonthly), round2(0.25 * esSMIMonthly)},
						Text: "200% of the minimum wage a month; +75% for the first dependent, +25% for each one after."},
					check("Remote work for companies outside Spain (Spanish clients at most 20% of the work)."),
					check("Working with the employer or clients for at least 3 months."),
					check("A university degree, or 3 years of experience in the field."),
					check("Private health insurance covering Spain, and a clean criminal record."),
				},
				Notes:   "Thresholds follow the minimum wage (SMI) of the year applied in.",
				Sources: []string{"https://www.boe.es/buscar/doc.php?id=BOE-A-2026-3815", "https://www.exteriores.gob.es/"},
			}
		}},
		{key: "non-lucrative", name: "Non-Lucrative Visa", category: CategoryPassive, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindIncome, Period: PeriodYear, Amount: 4 * esIPREM * 12, PerDependent: []float64{esIPREM * 12}, Group: "means",
						Text: "400% of the IPREM a year; +100% for each dependent. Passive income: working in Spain isn't allowed."},
					{Kind: KindSavings, Period: PeriodOnce, Amount: 4 * esIPREM * 12, PerDependent: []float64{esIPREM * 12}, Group: "means",
						Text: "Or the same amount held for the first year."},
					check("You won't work in Spain."),
					check("Private health insurance with no co-payments, a medical certificate and a clean criminal record."),
				},
				Notes:   "Consulates differ on whether savings alone are enough.",
				Sources: []string{"https://www.exteriores.gob.es/"},
			}
		}},
	},
	"PT": {
		{key: "d7", name: "D7 Passive Income Visa", category: CategoryPassive, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindIncome, Period: PeriodMonth, Amount: ptRMMG, PerDependent: []float64{0.5 * ptRMMG, 0.3 * ptRMMG}, Group: "means",
						Text: "The minimum wage a month of regular passive income (pensions, rents, dividends); +50% for a spouse, +30% for each child."},
					{Kind: KindSavings, Period: PeriodOnce, Amount: 12 * ptRMMG, PerDependent: []float64{12 * 0.5 * ptRMMG, 12 * 0.3 * ptRMMG}, Group: "means",
						Text: "Or a year of it held, usually in a Portuguese bank account."},
					check("Accommodation in Portugal for 12 months, health insurance and a clean criminal record."),
				},
				Notes:   "Many consulates ask for both the income and a year of it in a Portuguese bank.",
				Sources: []string{"https://vistos.mne.gov.pt/en/"},
			}
		}},
		{key: "d8", name: "D8 Digital Nomad Visa", category: CategoryNomad, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindIncome, Period: PeriodMonth, Amount: 4 * ptRMMG, MonthsProof: 3,
						Text: "4 times the minimum wage a month on average over the last 3 months, from work for employers or clients outside Portugal."},
					{Kind: KindSavings, Period: PeriodOnce, Amount: 12 * ptRMMG,
						Text: "A year of the minimum wage held, usually in a Portuguese bank account."},
					check("Remote work for employers or clients outside Portugal."),
					check("Accommodation, health insurance and a clean criminal record."),
				},
				Sources: []string{"https://vistos.mne.gov.pt/en/"},
			}
		}},
	},
	"DE": {
		{key: "blue-card", name: "EU Blue Card", category: CategoryWork, options: []Option{{Key: "job", Label: "Occupation", Choices: []Choice{
			{Value: "standard", Label: "Any qualified job"},
			{Value: "reduced", Label: "Shortage occupation, IT or recent graduate"},
		}}}, rule: func(opts map[string]string) Rule {
			amount := deBlueCard
			if opts["job"] == "reduced" {
				amount = deBlueCardReduced
			}
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodYear, Amount: amount, Text: "A job offer's gross salary a year."},
					check("A university degree recognized in Germany (IT specialists: 3 years of experience instead)."),
					check("A job offer in Germany matching the qualification."),
				},
				Notes:   "Thresholds follow the pension insurance's contribution ceiling and change every January.",
				Sources: []string{"https://www.make-it-in-germany.com/en/visa-residence/types/eu-blue-card"},
			}
		}},
		{key: "it-specialist", name: "IT Specialist (no degree)", category: CategoryWork, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodYear, Amount: deBlueCardReduced, Text: "A job offer's gross salary a year (the Blue Card's reduced threshold)."},
					check("At least 3 years of IT experience in the last 7, at a degree's level."),
					check("A job offer as an IT specialist in Germany."),
				},
				Sources: []string{"https://www.make-it-in-germany.com/en/visa-residence/types/eu-blue-card"},
			}
		}},
		{key: "opportunity-card", name: "Opportunity Card (Chancenkarte)", category: CategoryWork, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindBlocked, Period: PeriodOnce, Amount: 12 * deChancenMonth, Text: "A year of living costs in a blocked account (or a part time job's contract covering them)."},
					check("A vocational qualification or degree recognized in Germany, or 6 points."),
					check("German at A1 or English at B2."),
				},
				Sources: []string{"https://www.make-it-in-germany.com/en/visa-residence/types/job-search-opportunity-card"},
			}
		}},
		{key: "student", name: "Student Visa", category: CategoryStudent, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindBlocked, Period: PeriodOnce, Amount: 12 * deStudentMonth, Text: "A year of living costs in a blocked account (released a month at a time)."},
					check("Admission to a German university."),
					check("Health insurance."),
				},
				Sources: []string{"https://www.auswaertiges-amt.de/en/visa-service"},
			}
		}},
	},
	"CA": {
		{key: "express-entry", name: "Express Entry", category: CategoryWork, options: []Option{{Key: "program", Label: "Program", Choices: []Choice{
			{Value: "fsw", Label: "Federal Skilled Worker"},
			{Value: "cec", Label: "Canadian Experience Class"},
		}}}, rule: func(opts map[string]string) Rule {
			if opts["program"] == "cec" {
				return Rule{
					Currency: "CAD",
					Requirements: []Requirement{
						check("At least a year of skilled work experience in Canada in the last 3."),
						check("Language test at CLB 7 (or 5 for some jobs)."),
						check("A CRS score above the latest draws'."),
					},
					Notes:   "No proof of funds for the Canadian Experience Class.",
					Sources: []string{"https://www.canada.ca/en/immigration-refugees-citizenship/services/immigrate-canada/express-entry/documents/proof-funds.html"},
				}
			}
			return Rule{
				Currency: "CAD",
				Requirements: []Requirement{
					caFundsRequirement(),
					check("A year of skilled work experience, a language test at CLB 7 and an educational credential assessment."),
					check("A CRS score above the latest draws'."),
				},
				Notes:   "A valid job offer in Canada waives the proof of funds.",
				Sources: []string{"https://www.canada.ca/en/immigration-refugees-citizenship/services/immigrate-canada/express-entry/documents/proof-funds.html"},
			}
		}},
		{key: "start-up", name: "Start-up Visa", category: CategoryInvestor, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "CAD",
				Requirements: []Requirement{
					caFundsRequirement(),
					check("A letter of support from a designated organization."),
					check("Language test at CLB 5."),
				},
				Sources: []string{"https://www.canada.ca/en/immigration-refugees-citizenship/services/immigrate-canada/start-visa.html"},
			}
		}},
	},
	"NL": {
		{key: "highly-skilled-migrant", name: "Highly Skilled Migrant", category: CategoryWork, options: []Option{{Key: "age", Label: "Applicant", Choices: []Choice{
			{Value: "30plus", Label: "30 or older"},
			{Value: "under30", Label: "Under 30"},
			{Value: "graduate", Label: "Recent graduate / orientation year"},
		}}}, rule: func(opts map[string]string) Rule {
			amount := 5942.0
			switch opts["age"] {
			case "under30":
				amount = 4357
			case "graduate":
				amount = 3122
			}
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodMonth, Amount: amount, Text: "A job offer's gross salary a month, holiday allowance left out."},
					check("An employer recognized by the IND as a sponsor."),
				},
				Sources: []string{"https://ind.nl/en/required-amounts-income-requirements"},
			}
		}},
	},
	"GB": {
		{key: "skilled-worker", name: "Skilled Worker", category: CategoryWork, options: []Option{{Key: "rate", Label: "Threshold", Choices: []Choice{
			{Value: "general", Label: "General"},
			{Value: "new-entrant", Label: "New entrant / Immigration Salary List"},
		}}}, rule: func(opts map[string]string) Rule {
			amount := 41700.0
			if opts["rate"] == "new-entrant" {
				amount = 33400
			}
			return Rule{
				Currency: "GBP",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodYear, Amount: amount, Text: "A job offer's gross salary a year, or the occupation's going rate when higher."},
					check("A job offer from a licensed sponsor, at RQF level 6."),
					check("English at B1."),
				},
				Sources: []string{"https://www.gov.uk/skilled-worker-visa"},
			}
		}},
		{key: "global-talent", name: "Global Talent", category: CategoryWork, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "GBP",
				Requirements: []Requirement{
					check("An endorsement (digital technology: Tech Nation's successor body) as a leader or potential leader, or a prize on the list."),
					check("Evidence of recognition: product-led work, contributions beyond your job, open source, publications."),
				},
				Notes:   "No salary, job offer or funds asked.",
				Sources: []string{"https://www.gov.uk/global-talent"},
			}
		}},
	},
	"US": {
		{key: "h-1b", name: "H-1B Specialty Occupation", category: CategoryWork, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "USD",
				Requirements: []Requirement{
					check("A US employer's job offer in a specialty occupation, paying at least the prevailing wage of its level and area."),
					check("A bachelor's degree in the field (or its equivalent in experience)."),
					check("Selected in the yearly lottery (weighted by wage level since 2026), unless cap exempt."),
				},
				Notes:   "No fixed salary: the Department of Labor's prevailing wage. The employer pays the fees; a $100,000 payment on new petitions ordered in 2025 is tied up in court, so check it before filing.",
				Sources: []string{"https://www.uscis.gov/working-in-the-united-states/h-1b-specialty-occupations"},
			}
		}},
		{key: "o-1a", name: "O-1A Extraordinary Ability", category: CategoryWork, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "USD",
				Requirements: []Requirement{
					check("Sustained national or international acclaim: at least 3 of the 8 criteria (awards, memberships, press, judging others' work, original contributions, scholarly articles, a critical role, a high salary)."),
					check("A US employer or agent petitioning for you."),
				},
				Notes:   "No salary or funds asked; a high salary against the field's is one of the criteria. No lottery or cap.",
				Sources: []string{"https://www.uscis.gov/working-in-the-united-states/temporary-workers/o-1-visa-individuals-with-extraordinary-ability-or-achievement"},
			}
		}},
		{key: "eb-5", name: "EB-5 Immigrant Investor", category: CategoryInvestor, options: []Option{{Key: "area", Label: "Project", Choices: []Choice{
			{Value: "standard", Label: "Any area"},
			{Value: "tea", Label: "Targeted employment area / infrastructure"},
		}}}, rule: func(opts map[string]string) Rule {
			amount := 1_050_000.0
			if opts["area"] == "tea" {
				amount = 800_000
			}
			return Rule{
				Currency: "USD",
				Requirements: []Requirement{
					{Kind: KindInvestment, Period: PeriodOnce, Amount: amount, Text: "Invested at risk in a new US business (often through a regional center)."},
					check("The money's lawful source traced all the way."),
					check("The investment creates 10 full-time jobs."),
				},
				Notes:   "A green card. The amounts are adjusted for inflation every 5 years, next in January 2027.",
				Sources: []string{"https://www.uscis.gov/working-in-the-united-states/permanent-workers/eb-5-immigrant-investor-program"},
			}
		}},
		{key: "e-2", name: "E-2 Treaty Investor", category: CategoryInvestor, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "USD",
				Requirements: []Requirement{
					check("A citizen of a treaty country (Egypt is one)."),
					check("A substantial investment, at risk, in a US business you develop and direct (usually well over $100,000 for a small business)."),
					check("A real, operating business that does more than support you and your family."),
				},
				Notes:   "No fixed amount: substantial against the business' cost.",
				Sources: []string{"https://www.uscis.gov/working-in-the-united-states/temporary-workers/e-2-treaty-investors"},
			}
		}},
	},
	"AU": {
		{key: "skills-in-demand", name: "Skills in Demand (subclass 482)", category: CategoryWork, options: []Option{{Key: "stream", Label: "Stream", Choices: []Choice{
			{Value: "core", Label: "Core Skills"},
			{Value: "specialist", Label: "Specialist Skills"},
		}}}, rule: func(opts map[string]string) Rule {
			amount := 79423.0
			if opts["stream"] == "specialist" {
				amount = 146576
			}
			return Rule{
				Currency: "AUD",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodYear, Amount: amount, Text: "A job offer's salary a year (or the market rate, when higher)."},
					check("An approved sponsor's nomination; Core Skills: an occupation on the Core Skills Occupation List."),
					check("A year of relevant experience and English at the required level."),
				},
				Notes:   "Thresholds of nominations lodged from 1 July 2026 to 30 June 2027, indexed every July.",
				Sources: []string{"https://immi.homeaffairs.gov.au/visas/getting-a-visa/visa-listing/skills-in-demand-482"},
			}
		}},
	},
	"SG": {
		{key: "employment-pass", name: "Employment Pass", category: CategoryWork, options: []Option{{Key: "sector", Label: "Sector", Choices: []Choice{
			{Value: "general", Label: "Most sectors"},
			{Value: "financial", Label: "Financial services"},
		}}}, rule: func(opts map[string]string) Rule {
			amount := 5600.0
			if opts["sector"] == "financial" {
				amount = 6200
			}
			return Rule{
				Currency: "SGD",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodMonth, Amount: amount, Text: "A job offer's fixed salary a month, the minimum for young graduates (it rises with age)."},
					check("Enough COMPASS points (salary, qualifications, the firm's diversity and local hiring)."),
				},
				Notes:   "The qualifying salary grows with age, up to about twice the minimum in your mid-40s.",
				Sources: []string{"https://www.mom.gov.sg/passes-and-permits/employment-pass/eligibility"},
			}
		}},
	},
	"SE": {
		{key: "work-permit", name: "Work Permit", category: CategoryWork, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "SEK",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodMonth, Amount: 34470, Text: "A job offer's gross salary a month, at least 90% of Sweden's median salary."},
					check("Terms at least those of the Swedish collective agreement or the occupation's norm."),
					check("Health, life, occupational injury and pension insurance from the employer."),
				},
				Sources: []string{"https://www.migrationsverket.se/en/word-explanations/salary-requirements-for-a-work-permit.html"},
			}
		}},
	},
	"FR": {
		{key: "talent-passport", name: "Talent Passport (qualified employee)", category: CategoryWork, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodYear, Amount: 39582, Text: "A job offer's gross salary a year."},
					check("A master's from France, or a job at a recognized innovative company."),
				},
				Notes:   "France's EU Blue Card (a Talent Passport too) asks more salary but any degree.",
				Sources: []string{"https://www.service-public.fr/particuliers/vosdroits/F16922?lang=en"},
			}
		}},
	},
	"AT": {
		{key: "red-white-red", name: "Red-White-Red Card (other key workers)", category: CategoryWork, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodMonth, Amount: 3465, Text: "A job offer's gross salary a month (paid 14 times a year)."},
					check("At least 55 points (qualifications, experience, languages, age)."),
				},
				Notes:   "Skilled workers in shortage occupations only need the collective agreement's wage.",
				Sources: []string{"https://www.migration.gv.at/en/types-of-immigration/permanent-immigration/other-key-workers/"},
			}
		}},
	},
	"NZ": {
		{key: "skilled-migrant", name: "Skilled Migrant Category", category: CategoryWork, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "NZD",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodYear, Amount: nzMedianHour * 40 * 52, Text: "A job offer at the median wage (NZD 35.00 an hour), worked out at 40 hours a week."},
					check("6 points: a qualification, a profession's registration or income, plus skilled work in New Zealand."),
					check("A job (or offer) of at least 30 hours a week with an accredited employer."),
				},
				Notes:   "The median wage rises every March. A residence visa.",
				Sources: []string{"https://www.immigration.govt.nz/visas/skilled-migrant-category-resident-visa/"},
			}
		}},
	},
	"IE": {
		{key: "critical-skills", name: "Critical Skills Employment Permit", category: CategoryWork, options: []Option{{Key: "list", Label: "Occupation", Choices: []Choice{
			{Value: "listed", Label: "On the critical skills list"},
			{Value: "other", Label: "Not on the list"},
		}}}, rule: func(opts map[string]string) Rule {
			amount := 40904.0
			if opts["list"] == "other" {
				amount = 68911
			}
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodYear, Amount: amount, Text: "A job offer's gross salary a year."},
					check("A job offer of at least 2 years."),
					check("A relevant degree (for listed occupations)."),
				},
				Sources: []string{"https://enterprise.gov.ie/en/news-and-events/department-news/2025/december/20251202.html"},
			}
		}},
	},
	"EE": {
		{key: "digital-nomad", name: "Digital Nomad Visa", category: CategoryNomad, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindIncome, Period: PeriodMonth, Amount: 4500, MonthsProof: 6, Text: "At least this a month in each of the last 6 months (gross)."},
					check("Remote work for an employer abroad, your own company abroad, or mostly foreign clients."),
				},
				Notes:   "Estonia's pages differ on gross or net: gross used here.",
				Sources: []string{"https://www.e-resident.gov.ee/nomadvisa/"},
			}
		}},
		{key: "employment", name: "Residence Permit for Employment", category: CategoryWork, options: []Option{{Key: "level", Label: "Job", Choices: []Choice{
			{Value: "specialist", Label: "Specialist (1× the average wage)"},
			{Value: "top", Label: "Top specialist (1.5×)"},
		}}}, rule: func(opts map[string]string) Rule {
			amount := eeAvgWage
			if opts["level"] == "top" {
				amount = round2(1.5 * eeAvgWage)
			}
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodMonth, Amount: amount, Text: "A job offer's gross salary a month, at least Estonia's average wage times the coefficient."},
					check("A job offer from an employer registered in Estonia."),
				},
				Notes:   "The coefficients follow Statistics Estonia's average wage, reset every spring. Start-up founders and employees are exempt from the quota.",
				Sources: []string{"https://www.politsei.ee/en/instructions/residence-permit-for-employment"},
			}
		}},
		{key: "blue-card", name: "EU Blue Card", category: CategoryWork, options: []Option{{Key: "job", Label: "Occupation", Choices: []Choice{
			{Value: "standard", Label: "Any qualified job"},
			{Value: "shortage", Label: "Shortage field (IT, engineering)"},
		}}}, rule: func(opts map[string]string) Rule {
			amount := 37152.0
			if opts["job"] == "shortage" {
				amount = 35664
			}
			return Rule{
				Currency: "EUR",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodYear, Amount: amount, Text: "A job offer's gross salary a year."},
					check("A higher education degree, or 5 years of experience at its level (IT: 3 years)."),
					check("A job offer of at least 6 months."),
				},
				Sources: []string{"https://www.politsei.ee/en/instructions/eu-blue-card"},
			}
		}},
	},
	"AE": {
		{key: "remote-work", name: "Remote Work Visa", category: CategoryNomad, options: []Option{{Key: "as", Label: "Working as", Choices: []Choice{
			{Value: "employee", Label: "Employee"},
			{Value: "owner", Label: "Business owner"},
		}}}, rule: func(opts map[string]string) Rule {
			amount := 3500.0
			if opts["as"] == "owner" {
				amount = 5000
			}
			return Rule{
				Currency: "USD",
				Requirements: []Requirement{
					{Kind: KindIncome, Period: PeriodMonth, Amount: amount, MonthsProof: 6, Text: "At least this a month, shown by the last 6 months' bank statements."},
					check("Work for an employer (or your company) outside the UAE, for at least the visa's year."),
					check("Health insurance valid in the UAE."),
				},
				Sources: []string{"https://u.ae/en/information-and-services/visa-and-emirates-id/residence-visas/residence-visa-for-working-remotely-from-uae"},
			}
		}},
		{key: "golden-skilled", name: "Golden Visa (skilled professional)", category: CategoryWork, rule: func(map[string]string) Rule {
			return Rule{
				Currency: "AED",
				Requirements: []Requirement{
					{Kind: KindSalary, Period: PeriodMonth, Amount: 30000, Text: "A basic salary a month in the UAE."},
					check("A bachelor's degree and a job at the first or second occupational level."),
				},
				Notes:   "10 years, renewable. The salary rule is applied differently between emirates.",
				Sources: []string{"https://u.ae/en/information-and-services/visa-and-emirates-id/residence-visas/golden-visa"},
			}
		}},
	},
}

// BuiltinCountries are the ISO2 codes of the countries with visas here,
// sorted.
func BuiltinCountries() []string {
	out := make([]string, 0, len(builtins))
	for c := range builtins {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// HasBuiltin tells the country has visas here.
func HasBuiltin(country string) bool {
	_, ok := builtins[strings.ToUpper(country)]
	return ok
}

// BuiltinVisas are the country's visas here.
func BuiltinVisas(country string) []Visa {
	bs := builtins[strings.ToUpper(country)]
	out := make([]Visa, len(bs))
	for i, b := range bs {
		out[i] = Visa{Key: b.key, Name: b.name, Category: b.category}
	}
	return out
}

func findBuiltin(country, key string) (builtin, bool) {
	for _, b := range builtins[strings.ToUpper(country)] {
		if b.key == key {
			return b, true
		}
	}
	return builtin{}, false
}

// IsBuiltin tells the country has the visa here.
func IsBuiltin(country, key string) bool {
	_, ok := findBuiltin(country, key)
	return ok
}

// BuiltinOptions are the choices the visa's rules depend on.
func BuiltinOptions(country, key string) []Option {
	b, _ := findBuiltin(country, key)
	return b.options
}

// PickOptions keeps the values of opts that are one of the options'
// choices, the first choice for the others.
func PickOptions(options []Option, opts map[string]string) map[string]string {
	return payroll.PickOptions(options, opts)
}

// Builtin is the visa's rules here for the options, false when there are
// none.
func Builtin(country, key string, opts map[string]string) (*Rule, bool) {
	country = strings.ToUpper(country)
	b, ok := findBuiltin(country, key)
	if !ok {
		return nil, false
	}
	picked := PickOptions(b.options, opts)
	r := b.rule(picked)
	r.Country, r.Key, r.Name, r.Category = country, b.key, b.name, b.category
	r.Year, r.Source, r.Variant = builtinYear, SourceBuiltin, payroll.Variant(b.options, picked)
	return &r, true
}
