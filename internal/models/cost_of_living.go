package models

import (
	"math"
	"slices"
	"strings"
)

// ColGroup is a group of prices the cost of living calculator compares
// (an ICP expenditure group), an expense category's spending weighed by its
// group's prices.
type ColGroup string

const (
	ColFood          ColGroup = "food"
	ColAlcohol       ColGroup = "alcohol"
	ColClothing      ColGroup = "clothing"
	ColHousing       ColGroup = "housing"
	ColFurnishings   ColGroup = "furnishings"
	ColHealth        ColGroup = "health"
	ColTransport     ColGroup = "transport"
	ColCommunication ColGroup = "communication"
	ColRecreation    ColGroup = "recreation"
	ColEducation     ColGroup = "education"
	ColRestaurants   ColGroup = "restaurants"
	ColMisc          ColGroup = "misc"
	// ColAverage weighs a category by the prices of everything (the average
	// household's basket): what a category not mapped to a group gets.
	ColAverage ColGroup = "avg"
)

// ColGroupInfo describes a group: its name and its ICP 2021 series code.
type ColGroupInfo struct {
	Group ColGroup
	Label string
	ICP   string
}

// ColGroups are the groups, in the ICP's order.
var ColGroups = []ColGroupInfo{
	{ColFood, "Groceries", "1101000"},
	{ColAlcohol, "Alcohol & tobacco", "1102000"},
	{ColClothing, "Clothing & shoes", "1103000"},
	{ColHousing, "Housing & utilities", "9060000"},
	{ColFurnishings, "Home & furnishings", "1105000"},
	{ColHealth, "Health", "9080000"},
	{ColTransport, "Transport", "1107000"},
	{ColCommunication, "Phone & internet", "1108000"},
	{ColRecreation, "Recreation & culture", "9110000"},
	{ColEducation, "Education", "9120000"},
	{ColRestaurants, "Restaurants & hotels", "1111000"},
	{ColMisc, "Personal care & other", "9140000"},
}

// ColICPTotal is the ICP 2021 series of the households' whole consumption,
// the groups' prices are taken relative to.
const ColICPTotal = "9100000"

// ColAverageSeries is the series key of the World Bank's household
// consumption PPP (the yearly, up to date level).
const ColAverageSeries = "avg"

// ColGroupLabel names a group ("Other (average prices)" for ColAverage).
func ColGroupLabel(g ColGroup) string {
	for _, i := range ColGroups {
		if i.Group == g {
			return i.Label
		}
	}
	return "Other (average prices)"
}

// ValidColGroup tells g is a group or ColAverage.
func ValidColGroup(g ColGroup) bool {
	return g == ColAverage || slices.ContainsFunc(ColGroups, func(i ColGroupInfo) bool { return i.Group == g })
}

// colKeywords guess a category's group from its name, the first group with a
// keyword in it winning (restaurants before food: "fast food").
var colKeywords = []struct {
	group ColGroup
	words []string
}{
	{ColRestaurants, []string{"restaurant", "dining", "eating out", "eat out", "takeaway", "take away", "takeout", "delivery", "fast food", "cafe", "café", "coffee", "hotel", "travel", "trip", "vacation", "holiday", "food out", "lunch", "dinner", "breakfast", "snack"}},
	{ColFood, []string{"grocer", "supermarket", "market", "food", "bakery", "butcher", "vegetable", "fruit", "meat", "water bottle", "drinks"}},
	{ColAlcohol, []string{"alcohol", "beer", "wine", "liquor", "tobacco", "cigar", "smok", "vape", "shisha"}},
	{ColClothing, []string{"cloth", "shoe", "fashion", "apparel", "wear", "outfit", "laundry", "dry clean"}},
	{ColHousing, []string{"rent", "housing", "house", "home loan", "mortgage", "electric", "water", "gas", "utilit", "maintenance", "repair", "building", "compound", "hoa"}},
	{ColFurnishings, []string{"furnitur", "furnish", "appliance", "household", "cleaning", "home", "kitchen", "decor", "maid", "nanny", "housekeep"}},
	{ColHealth, []string{"health", "medic", "doctor", "pharma", "hospital", "clinic", "dental", "dentist", "drug", "insurance", "gym", "fitness", "therapy"}},
	{ColTransport, []string{"transport", "car", "fuel", "petrol", "gasoline", "uber", "careem", "taxi", "metro", "bus", "train", "parking", "toll", "commute", "ride", "vehicle", "bike", "flight", "airline"}},
	{ColCommunication, []string{"phone", "mobile", "internet", "telecom", "wifi", "broadband", "sim", "data plan", "postal"}},
	{ColRecreation, []string{"entertain", "fun", "leisure", "hobby", "game", "gaming", "movie", "cinema", "music", "concert", "subscription", "streaming", "netflix", "spotify", "book", "sport", "outing", "gift", "pet", "toy", "electronic", "gadget"}},
	{ColEducation, []string{"educat", "school", "tuition", "course", "universit", "college", "class", "learning", "training", "study"}},
	{ColMisc, []string{"personal care", "beauty", "salon", "barber", "haircut", "cosmetic", "hygiene", "toiletr", "care", "charity", "donation", "bank fee", "fee", "service"}},
}

// GuessColGroup is the group a category's name suggests, ColAverage when
// none does.
func GuessColGroup(name string) ColGroup {
	n := strings.ToLower(name)
	for _, k := range colKeywords {
		for _, w := range k.words {
			if colKeywordIn(n, w) {
				return k.group
			}
		}
	}
	return ColAverage
}

// colKeywordIn tells w starts a word of n ("car" in "car loan", not in
// "care"; short keywords must be whole words).
func colKeywordIn(n, w string) bool {
	for i := 0; ; {
		j := strings.Index(n[i:], w)
		if j < 0 {
			return false
		}
		j += i
		startOK := j == 0 || !isLetter(n[j-1])
		end := j + len(w)
		endOK := len(w) > 3 || end == len(n) || !isLetter(n[end])
		if startOK && endOK {
			return true
		}
		i = j + 1
	}
}

func isLetter(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 0x80
}

// ColPrices are a place's prices: its level (local currency per
// international dollar, today) and each group's prices relative to it.
type ColPrices struct {
	Level float64
	// Rel are the groups' prices over the level, 1 = as expensive as the
	// place's average.
	Rel map[ColGroup]float64
}

// ColRatios are how many units of the target's currency buy there what one
// unit of the home currency buys at home: overall and per group.
type ColRatios struct {
	Avg    float64
	Groups map[ColGroup]float64
}

// Ratios are the target's prices against home's, nil when either level is
// missing. A group either lacks takes the average's ratio.
func (home ColPrices) Ratios(target ColPrices) *ColRatios {
	if home.Level <= 0 || target.Level <= 0 {
		return nil
	}
	r := &ColRatios{Avg: target.Level / home.Level, Groups: map[ColGroup]float64{}}
	for _, g := range ColGroups {
		h, t := home.Rel[g.Group], target.Rel[g.Group]
		if h > 0 && t > 0 {
			r.Groups[g.Group] = r.Avg * t / h
		}
	}
	return r
}

// Group is a group's ratio, the average's when it has none.
func (r *ColRatios) Group(g ColGroup) float64 {
	if v := r.Groups[g]; v > 0 {
		return v
	}
	return r.Avg
}

// ColGroupResult is a group's part in the user's basket.
type ColGroupResult struct {
	Group ColGroup
	// Share of the basket (0–1).
	Share float64
	// Ratio is the group's (target currency per home currency).
	Ratio float64
	// VsAverage is how the group's prices there compare with the average
	// ones, against home: 0.2 = 20% pricier than everything else.
	VsAverage float64
	// Home is the monthly spending at home (home currency), There what it
	// comes to there (target currency).
	Home, There float64
}

// ColResult is the cost of living there of an amount spent at home.
type ColResult struct {
	Amount float64
	// AvgRatio weighs by the average household's basket, MineRatio by the
	// user's spending (equal to AvgRatio without any).
	AvgRatio, MineRatio float64
	// Average and Mine are the amount there (target currency).
	Average, Mine float64
	// HasBasket tells the user had spending to weigh by.
	HasBasket bool
	// Diff is how much more (or less, negative) the user's basket costs
	// there than the average's: Mine / Average − 1.
	Diff float64
	// Driver is the group moving Mine away from Average the most, in the
	// direction of Diff ("" when none does).
	Driver ColGroup
	// Market is the exchange rate (target per home currency), 0 unknown;
	// then PriceLevel / MinePriceLevel are 0 too.
	Market float64
	// PriceLevel is how expensive the target is against home at the
	// market rate: 1.5 = 50% pricier.
	PriceLevel, MinePriceLevel float64
	// Groups are the user's basket, biggest share first.
	Groups []ColGroupResult
	// MonthlySpend is the user's monthly spending weighed (home currency).
	MonthlySpend float64
}

// ComputeCol is what amount (home currency) comes to there, weighed by the
// average basket and by spend: the user's monthly spending per group (home
// currency). market is the exchange rate (target per home), 0 unknown.
func ComputeCol(amount float64, r ColRatios, spend map[ColGroup]float64, market float64) ColResult {
	res := ColResult{Amount: amount, AvgRatio: r.Avg, MineRatio: r.Avg, Market: market}

	var total float64
	for g, v := range spend {
		if v > 0 && ValidColGroup(g) {
			total += v
		}
	}
	if total > 0 {
		res.HasBasket = true
		res.MonthlySpend = total
		var mine float64
		for g, v := range spend {
			if v <= 0 || !ValidColGroup(g) {
				continue
			}
			ratio := r.Group(g)
			mine += v / total * ratio
			gr := ColGroupResult{Group: g, Share: v / total, Ratio: ratio, Home: v, There: v * ratio}
			if r.Avg > 0 {
				gr.VsAverage = ratio/r.Avg - 1
			}
			res.Groups = append(res.Groups, gr)
		}
		res.MineRatio = mine
		slices.SortFunc(res.Groups, func(a, b ColGroupResult) int {
			if a.Share != b.Share {
				if a.Share > b.Share {
					return -1
				}
				return 1
			}
			return strings.Compare(string(a.Group), string(b.Group))
		})
	}

	res.Average = amount * res.AvgRatio
	res.Mine = amount * res.MineRatio
	if res.AvgRatio > 0 {
		res.Diff = res.MineRatio/res.AvgRatio - 1
	}
	if math.Abs(res.Diff) >= 0.005 {
		var best float64
		for _, g := range res.Groups {
			// Its pull on Mine, signed like Diff.
			pull := g.Share * g.VsAverage
			if pull*res.Diff > 0 && math.Abs(pull) > best && g.Group != ColAverage {
				best, res.Driver = math.Abs(pull), g.Group
			}
		}
	}
	if market > 0 {
		res.PriceLevel = res.AvgRatio / market
		res.MinePriceLevel = res.MineRatio / market
	}
	return res
}

// DriverResult is the group of Driver in Groups.
func (r ColResult) DriverResult() (ColGroupResult, bool) {
	for _, g := range r.Groups {
		if g.Group == r.Driver && r.Driver != "" {
			return g, true
		}
	}
	return ColGroupResult{}, false
}

// ColCategory is an expense category in the user's basket.
type ColCategory struct {
	ID    string
	Name  string
	Color string
	// Group is the picked one, else Guessed.
	Group   ColGroup
	Guessed bool
	// Monthly spending (home currency).
	Monthly float64
}

// ColSpendByGroup sums the categories' monthly spending per group.
func ColSpendByGroup(cats []ColCategory) map[ColGroup]float64 {
	out := map[ColGroup]float64{}
	for _, c := range cats {
		if c.Monthly > 0 {
			out[c.Group] += c.Monthly
		}
	}
	return out
}

// ColCityEstimate is Gemini's estimate of a city's prices against another's:
// price levels at the market rate (1.3 = 30% pricier there), overall and per
// group, and the rent of a one bedroom flat in the center of each.
type ColCityEstimate struct {
	// HomeCity and TargetCity are the cities' usual English names, as the
	// model understood what was typed ("" when it couldn't tell).
	HomeCity   string               `json:"home_city,omitempty"`
	TargetCity string               `json:"target_city,omitempty"`
	Overall    float64              `json:"overall"`
	Groups     map[ColGroup]float64 `json:"groups"`
	// Rent of a one bedroom flat in the center, each in its city's currency.
	RentHome   float64 `json:"rent_home,omitempty"`
	RentTarget float64 `json:"rent_target,omitempty"`
	Notes      string  `json:"notes,omitempty"`
	// Sources are the pages the estimate was grounded on.
	Sources []ColSource `json:"sources,omitempty"`
	Model   string      `json:"model,omitempty"`
}

// ColSource is a web page an estimate was grounded on.
type ColSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Ratios turn the estimate's price levels into ratios at the market rate
// (target per home currency), nil without a rate or overall level.
func (e *ColCityEstimate) Ratios(market float64) *ColRatios {
	if e == nil || market <= 0 || e.Overall <= 0 {
		return nil
	}
	r := &ColRatios{Avg: e.Overall * market, Groups: map[ColGroup]float64{}}
	for _, g := range ColGroups {
		if v := e.Groups[g.Group]; v > 0 {
			r.Groups[g.Group] = v * market
		}
	}
	return r
}
