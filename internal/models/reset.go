package models

import "net/url"

// ResetEntity is a kind of data the user can wipe from their account.
type ResetEntity string

const (
	ResetTransactions   ResetEntity = "transactions"
	ResetRecurrent      ResetEntity = "recurrent"
	ResetSMS            ResetEntity = "sms"
	ResetCategories     ResetEntity = "categories"
	ResetCounterparties ResetEntity = "counterparties"
	ResetGoals          ResetEntity = "goals"
	ResetAccounts       ResetEntity = "accounts"
	ResetWishlist       ResetEntity = "wishlist"
	ResetDevices        ResetEntity = "devices"
	ResetSettings       ResetEntity = "settings"
)

// ResetConfirmation has to be typed by the user to reset.
const ResetConfirmation = "RESET"

type ResetEntityInfo struct {
	Entity      ResetEntity
	Label       string
	Description string
	// Requires are the entities deleted along with this one.
	Requires []ResetEntity
}

var ResetEntities = []ResetEntityInfo{
	{ResetTransactions, "Transactions", "Every transaction, scheduled ones and reimbursements included. Kept accounts go back to a 0 balance.", nil},
	{ResetRecurrent, "Recurring transactions", "The recurring templates. Transactions they already made are kept unless deleted too.", nil},
	{ResetSMS, "SMS messages", "Every received bank SMS.", nil},
	{ResetCategories, "Categories", "Categories and their budgets. Kept transactions lose their category.", nil},
	{ResetCounterparties, "Counterparties", "People and merchants, with their aliases and icons.", nil},
	{ResetGoals, "Goals", "Savings goals. Kept transactions lose their goal.", nil},
	{ResetAccounts, "Accounts", "Every account and gam'yaa, with all their transactions and recurring transactions.", []ResetEntity{ResetTransactions, ResetRecurrent}},
	{ResetWishlist, "Wishlist", "The items on the decide page's wishlist.", nil},
	{ResetDevices, "Devices", "Linked phones, they'll stop forwarding SMS until signed in again.", nil},
	{ResetSettings, "Settings", "Default currency and account, shown currencies and work settings (salary, hours).", nil},
}

// ResetOptions is the set of entities to delete.
type ResetOptions map[ResetEntity]bool

// ParseResetOptions reads the checked entities, adding the ones they require.
func ParseResetOptions(form url.Values) ResetOptions {
	opts := ResetOptions{}
	for _, v := range form["entity"] {
		for _, info := range ResetEntities {
			if string(info.Entity) == v {
				opts[info.Entity] = true
			}
		}
	}
	opts.Normalize()
	return opts
}

// Normalize adds the entities required by the selected ones.
func (o ResetOptions) Normalize() {
	for _, info := range ResetEntities {
		if !o[info.Entity] {
			continue
		}
		for _, req := range info.Requires {
			o[req] = true
		}
	}
}

func (o ResetOptions) Has(e ResetEntity) bool { return o[e] }

func (o ResetOptions) IsEmpty() bool {
	for _, v := range o {
		if v {
			return false
		}
	}
	return true
}

// IsAll is whether everything gets deleted, a full reset.
func (o ResetOptions) IsAll() bool {
	for _, info := range ResetEntities {
		if !o[info.Entity] {
			return false
		}
	}
	return true
}
