package payroll

// The built in rules, an employee's, of 2026 (the UK's 2026/27 tax year).
// Each lists where its figures come from: check them again every year.

// egypt: Income Tax Law 91/2005 as amended by Law 7/2024 (brackets, the
// higher incomes losing the lower ones, EGP 20,000 personal exemption) and
// the Social Insurance Law 148/2019 (11% of the insurable wage, EGP 2,700 –
// 16,700 a month from January 2026), plus the martyrs' fund's 0.05%.
var egypt = builtin{
	rules: func(map[string]string) Rules {
		top := []Bracket{{400_000, 0.225}, {1_200_000, 0.25}, {0, 0.275}}
		return Rules{
			Year: 2026, Currency: "EGP",
			Brackets: append([]Bracket{{40_000, 0}, {55_000, 0.10}, {70_000, 0.15}, {200_000, 0.20}}, top...),
			Tables: []Table{
				{Above: 600_000, Brackets: append([]Bracket{{55_000, 0.10}, {70_000, 0.15}, {200_000, 0.20}}, top...)},
				{Above: 700_000, Brackets: append([]Bracket{{70_000, 0.15}, {200_000, 0.20}}, top...)},
				{Above: 800_000, Brackets: append([]Bracket{{200_000, 0.20}}, top...)},
				{Above: 900_000, Brackets: top},
				{Above: 1_200_000, Brackets: []Bracket{{1_200_000, 0.25}, {0, 0.275}}},
			},
			Deductions: []Deduction{{Name: "Personal exemption", Amount: 20_000}},
			Contributions: []Contribution{
				{Name: "Social insurance", Rate: 0.11, Floor: 2_700, Cap: 16_700, Monthly: true, Deductible: true},
				{Name: "Martyrs' fund", Rate: 0.0005},
			},
			Notes: "Social insurance is on the insurable wage, taken here as the gross within its limits.",
			Sources: []string{
				"https://taxsummaries.pwc.com/egypt/individual/taxes-on-personal-income",
				"https://taxsummaries.pwc.com/egypt/individual/other-taxes",
			},
		}
	},
}

// spain: IRPF 2026 general scale (state + the standard regional half; each
// region sets its own), the €5,550 personal minimum as a credit, the work
// income reduction (art. 20 LIRPF) and €2,000 of other expenses; Social
// Security employee share (common contingencies 4.70%, unemployment 1.55%,
// training 0.10%, MEI 0.15%) on the base up to €5,101.20 a month.
var spain = builtin{
	options: []Option{{Key: "pays", Label: "Pays a year", Choices: []Choice{{"12", "12 pays"}, {"14", "14 pays (two extra)"}}}},
	rules: func(opts map[string]string) Rules {
		pays := 12
		if opts["pays"] == "14" {
			pays = 14
		}
		return Rules{
			Year: 2026, Currency: "EUR", PaysPerYear: pays,
			Brackets:  []Bracket{{12_450, 0.19}, {20_200, 0.24}, {35_200, 0.30}, {60_000, 0.37}, {300_000, 0.45}, {0, 0.47}},
			Allowance: Allowance{Amount: 5_550, AsCredit: true},
			WorkReduction: []Step{
				{From: 0, UpTo: 14_852, Amount: 7_302},
				{From: 14_852, UpTo: 17_673.52, Amount: 7_302, Slope: 1.75},
				{From: 17_673.52, UpTo: 19_747.5, Amount: 2_364.34, Slope: 1.14},
			},
			Deductions: []Deduction{{Name: "Other expenses", Amount: 2_000}},
			Contributions: []Contribution{
				{Name: "Social Security", Rate: 0.065, Floor: 1_424.40, Cap: 5_101.20, Monthly: true, Deductible: true},
			},
			Notes: "Regional scales differ: the standard one is used. Withholding (retención) may differ from the year's tax.",
			Sources: []string{
				"https://sede.agenciatributaria.gob.es/Sede/irpf.html",
				"https://www.seg-social.es/wps/portal/wss/internet/Trabajadores/CotizacionRecaudacionTrabajadores/36537",
			},
		}
	},
}

// saudi: no income tax; GOSI for Saudis: annuities (9% on the legacy system,
// 10% on the new one from July 2026, for who joined from July 2024) + SANED
// 0.75%, on up to SAR 45,000 a month. Expats pay nothing (the employer pays
// occupational hazards).
var saudi = builtin{
	options: []Option{{Key: "nationality", Label: "You are", Choices: []Choice{
		{"expat", "An expat"}, {"saudi", "Saudi (joined before July 2024)"}, {"saudi_new", "Saudi (joined from July 2024)"},
	}}},
	rules: func(opts map[string]string) Rules {
		r := Rules{
			Year: 2026, Currency: "SAR",
			Brackets: []Bracket{{0, 0}},
			Sources:  []string{"https://www.gosi.gov.sa/en/SystemsAndRegulations/SocialInsuranceLaw"},
		}
		switch opts["nationality"] {
		case "saudi":
			r.Contributions = []Contribution{
				{Name: "GOSI annuities", Rate: 0.09, Floor: 1_500, Cap: 45_000, Monthly: true},
				{Name: "SANED (unemployment)", Rate: 0.0075, Floor: 1_500, Cap: 45_000, Monthly: true},
			}
		case "saudi_new":
			r.Contributions = []Contribution{
				{Name: "GOSI annuities", Rate: 0.10, Floor: 1_500, Cap: 45_000, Monthly: true},
				{Name: "SANED (unemployment)", Rate: 0.0075, Floor: 1_500, Cap: 45_000, Monthly: true},
			}
			r.Notes = "The new system's annuities rate goes up by 0.5% each July until 11% in 2028."
		default:
			r.Notes = "Expats pay no income tax nor social insurance."
		}
		return r
	},
}

// uae: no income tax; Emiratis pay 11% of the contribution account salary
// to the GPSSA (up to AED 70,000 a month), expats nothing.
var uae = builtin{
	options: []Option{{Key: "nationality", Label: "You are", Choices: []Choice{{"expat", "An expat"}, {"national", "An Emirati"}}}},
	rules: func(opts map[string]string) Rules {
		r := Rules{
			Year: 2026, Currency: "AED",
			Brackets: []Bracket{{0, 0}},
			Sources:  []string{"https://gpssa.gov.ae/pages/en/services/contributions"},
		}
		if opts["nationality"] == "national" {
			r.Contributions = []Contribution{{Name: "Pension (GPSSA)", Rate: 0.11, Floor: 3_000, Cap: 70_000, Monthly: true}}
			r.Notes = "The pension is on the contribution account salary, taken here as the gross within its limits. Abu Dhabi's fund has its own rules."
		} else {
			r.Notes = "Expats pay no income tax nor pension contributions."
		}
		return r
	},
}

// uk: 2026/27, £12,570 personal allowance tapered over £100,000; rUK bands
// (20/40/45%) or Scotland's six; employee class 1 NI 8% up to £50,270, 2%
// over.
var uk = builtin{
	options: []Option{{Key: "region", Label: "Where", Choices: []Choice{{"ruk", "England, Wales, N. Ireland"}, {"scotland", "Scotland"}}}},
	rules: func(opts map[string]string) Rules {
		// Bands are of the taxable income, after the allowance.
		brackets := []Bracket{{37_700, 0.20}, {125_140, 0.40}, {0, 0.45}}
		sources := []string{"https://www.gov.uk/government/publications/rates-and-allowances-income-tax/income-tax-rates-and-allowances-current-and-past"}
		if opts["region"] == "scotland" {
			brackets = []Bracket{
				{16_537 - 12_570, 0.19}, {29_526 - 12_570, 0.20}, {43_662 - 12_570, 0.21},
				{75_000 - 12_570, 0.42}, {125_140, 0.45}, {0, 0.48},
			}
			sources = append(sources, "https://www.gov.scot/publications/scottish-income-tax-technical-factsheet/")
		}
		return Rules{
			Year: 2026, Currency: "GBP",
			Brackets:  brackets,
			Allowance: Allowance{Amount: 12_570, TaperFrom: 100_000, TaperRate: 0.5},
			Contributions: []Contribution{
				{Name: "National Insurance", Bands: []Bracket{{12_570, 0}, {50_270, 0.08}, {0, 0.02}}},
			},
			Notes:   "Tax year 2026/27. The higher bands are of the income over the allowance, as HMRC words them; with the allowance tapered away they start sooner.",
			Sources: sources,
		}
	},
}

// us: 2026 federal brackets and standard deduction (Rev. Proc. 2025-32),
// Social Security 6.2% up to $184,500, Medicare 1.45% + 0.9% over
// $200,000 ($250,000 married). No state tax: the form's "other tax" stands
// in for it.
var us = builtin{
	options: []Option{{Key: "filing", Label: "Filing", Choices: []Choice{{"single", "Single"}, {"married", "Married, jointly"}}}},
	rules: func(opts map[string]string) Rules {
		r := Rules{
			Year: 2026, Currency: "USD",
			Brackets:   []Bracket{{12_400, 0.10}, {50_400, 0.12}, {105_700, 0.22}, {201_775, 0.24}, {256_225, 0.32}, {640_600, 0.35}, {0, 0.37}},
			Deductions: []Deduction{{Name: "Standard deduction", Amount: 16_100}},
			Contributions: []Contribution{
				{Name: "Social Security", Rate: 0.062, Cap: 184_500},
				{Name: "Medicare", Bands: []Bracket{{200_000, 0.0145}, {0, 0.0235}}},
			},
			Notes: "Federal only: add your state's tax as other tax. Married: both incomes together.",
			Sources: []string{
				"https://www.irs.gov/newsroom/irs-releases-tax-inflation-adjustments-for-tax-year-2026-including-amendments-from-the-one-big-beautiful-bill",
				"https://www.ssa.gov/oact/cola/cbb.html",
			},
		}
		if opts["filing"] == "married" {
			r.Brackets = []Bracket{{24_800, 0.10}, {100_800, 0.12}, {211_400, 0.22}, {403_550, 0.24}, {512_450, 0.32}, {768_700, 0.35}, {0, 0.37}}
			r.Deductions[0].Amount = 32_200
			r.Contributions[1].Bands = []Bracket{{250_000, 0.0145}, {0, 0.0235}}
		}
		return r
	},
}

// canada: 2026 federal brackets (14% from 2026), basic personal amount
// $16,452 clawed back to $14,829 between $181,440 and $258,482, Canada
// employment amount $1,501; CPP 5.95% between $3,500 and $74,600 (4.95%
// base credited, 1% enhanced deducted) and CPP2 4% up to $85,000
// (deducted); EI 1.63% up to $68,900 (credited). Then the province's tax:
// Ontario (with its surtax and health premium), British Columbia or
// Alberta, its credits at its lowest rate. Quebec is apart (QPP, QPIP, the
// federal abatement): its rules are left to the AI lookup.
var canada = builtin{
	options: []Option{{Key: "province", Label: "Province", Choices: []Choice{
		{"on", "Ontario"}, {"bc", "British Columbia"}, {"ab", "Alberta"},
	}}},
	rules: func(opts map[string]string) Rules {
		r := Rules{
			Year: 2026, Currency: "CAD", TaxName: "Federal",
			Brackets: []Bracket{{58_523, 0.14}, {117_045, 0.205}, {181_440, 0.26}, {258_482, 0.29}, {0, 0.33}},
			Credits: []Credit{
				{Name: "Basic personal amount", Amount: 16_452, TaperFrom: 181_440, TaperTo: 258_482, Floor: 14_829},
				{Name: "Canada employment amount", Amount: 1_501},
			},
			Contributions: []Contribution{
				{Name: "CPP (base)", Bands: []Bracket{{3_500, 0}, {74_600, 0.0495}, {0, 0}}, Credit: true},
				{Name: "CPP (enhanced)", Bands: []Bracket{{3_500, 0}, {74_600, 0.01}, {0, 0}}, Deductible: true},
				{Name: "CPP2", Bands: []Bracket{{74_600, 0}, {85_000, 0.04}, {0, 0}}, Deductible: true},
				{Name: "EI", Rate: 0.0163, Cap: 68_900, Credit: true},
			},
			Notes: "Federal tax plus the province's, an employee's basic credits only. Quebec has its own rules: look them up with AI (region Quebec).",
			Sources: []string{
				"https://www.canada.ca/en/revenue-agency/services/tax/individuals/tax-rates-brackets/current-year.html",
				"https://www.canada.ca/en/revenue-agency/services/forms-publications/payroll/t4127-payroll-deductions-formulas/t4127-jan/t4127-jan-payroll-deductions-formulas-computer-programs.html",
			},
		}
		switch opts["province"] {
		case "bc":
			r.Layers = []Layer{{
				Name:     "British Columbia",
				Brackets: []Bracket{{50_363, 0.056}, {100_728, 0.077}, {115_648, 0.105}, {140_430, 0.1229}, {190_405, 0.147}, {265_545, 0.168}, {0, 0.205}},
				Credits:  []Credit{{Name: "Basic personal amount", Amount: 13_216}},
			}}
			r.Notes += " B.C.'s low income tax reduction is left out."
			r.Sources = append(r.Sources, "https://www2.gov.bc.ca/gov/content/taxes/income-taxes/personal/tax-rates")
		case "ab":
			r.Layers = []Layer{{
				Name:     "Alberta",
				Brackets: []Bracket{{61_200, 0.08}, {154_259, 0.10}, {185_111, 0.12}, {246_813, 0.13}, {370_220, 0.14}, {0, 0.15}},
				Credits:  []Credit{{Name: "Basic personal amount", Amount: 22_769}},
			}}
			r.Sources = append(r.Sources, "https://www.alberta.ca/personal-income-tax")
		default:
			r.Layers = []Layer{{
				Name:     "Ontario",
				Brackets: []Bracket{{53_891, 0.0505}, {107_785, 0.0915}, {150_000, 0.1116}, {220_000, 0.1216}, {0, 0.1316}},
				Credits:  []Credit{{Name: "Basic personal amount", Amount: 12_989}},
				// 20% of the Ontario tax over $5,818, 36% more over $7,446.
				Surtax: []Bracket{{5_818, 0}, {7_446, 0.20}, {0, 0.56}},
			}}
			// The health premium's steps, as bands of the pay.
			r.Contributions = append(r.Contributions, Contribution{Name: "Ontario Health Premium", Bands: []Bracket{
				{20_000, 0}, {25_000, 0.06}, {36_000, 0}, {38_500, 0.06}, {48_000, 0}, {48_600, 0.25},
				{72_000, 0}, {72_600, 0.25}, {200_000, 0}, {200_600, 0.25}, {0, 0},
			}})
			r.Notes += " Ontario's low income tax reduction is left out."
			r.Sources = append(r.Sources, "https://www.ontario.ca/document/personal-income-tax-rates-and-credits")
		}
		return r
	},
}
