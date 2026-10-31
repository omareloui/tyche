package smsparser

import "github.com/omareloui/tyche/internal/models"

// SeedVersion is bumped when Seeds changes, so the shared database gets the
// new templates (the existing ones, by seed key, are left as they are).
const SeedVersion = 1

var (
	hsbc     = []string{"HSBC"}
	cib      = []string{"CIB"}
	aaib     = []string{"AAIB"}
	arabBank = []string{"Arab Bank"}
)

func seed(key, bank string, senders []string, kind models.TxKind, name, body, sample string, fixed map[string]string) models.SMSTemplate {
	action := models.SMSTemplateTransaction
	if kind == models.NilTxKind {
		action = models.SMSTemplateIgnore
	}
	return models.SMSTemplate{
		Scope:     models.SMSTemplateShared,
		SeedKey:   key,
		Name:      name,
		Bank:      bank,
		Senders:   senders,
		Action:    action,
		TxKind:    kind,
		Body:      body,
		Sample:    sample,
		Fixed:     fixed,
		IsEnabled: true,
	}
}

func ignore(key, bank string, senders []string, name, body, sample string) models.SMSTemplate {
	return seed(key, bank, senders, models.NilTxKind, name, body, sample, nil)
}

// Seeds are the templates of the formats known so far, seeded into the shared
// database.
func Seeds() []models.SMSTemplate {
	const (
		expense  = models.ExpenseTxKind
		income   = models.IncomeTxKind
		transfer = models.TransferTxKind
		refund   = models.RefundTxKind
	)
	toCash := map[string]string{"to_account_id": "Cash"}
	fromCash := map[string]string{"account_id": "Cash"}

	return []models.SMSTemplate{
		// HSBC
		seed("hsbc.en.atm_withdrawal", "HSBC", hsbc, transfer, "ATM withdrawal",
			`From HSBC: {{date}} ATM Cash Withdrawal from {{account_id}} {{currency}} {{amount}} Your available balance is {{available_currency}} {{available_amount}}`,
			`From HSBC: 28JUL25 ATM Cash Withdrawal from 123-456***-789 EGP 1,000.00- Your available balance is EGP 27,905.35`,
			toCash),
		seed("hsbc.en.atm_reversal", "HSBC", hsbc, transfer, "ATM withdrawal reversal",
			`From HSBC: {{date}} ATM Reversal of Cash Withdrawal from {{to_account_id}} {{currency}} {{amount}} Your available balance is {{available_currency}} {{available_amount}}`,
			`From HSBC: 28JUL25 ATM Reversal of Cash Withdrawal from 123-456***-789 EGP 1,000.00+ Your available balance is EGP 28,905.35`,
			fromCash),
		seed("hsbc.en.cc_purchase", "HSBC", hsbc, expense, "Credit card purchase",
			`Your Credit Card ending with *** {{account_id}} has been used for {{currency}} {{amount}} on {{date}} at {{counterparty:merchant}}. Your available limit is {{available_currency}} {{available_amount}}{{*}}`,
			`Your Credit Card ending with *** 1234 has been used for USD 30.00 on 31/10/2025 at PAYMENT PROCESSOR. Your available limit is USD 5000.00`,
			nil),
		seed("hsbc.en.debit_purchase", "HSBC", hsbc, expense, "Debit card purchase",
			`From HSBC: {{date}} {{counterparty:merchant}} Purchase from {{account_id}} {{currency}} {{amount}} Your available balance is {{available_currency}} {{available_amount}}`,
			`From HSBC: 28JUL25 MERCHANT1*LOCALLY Purchase from 123-456***-789 USD 125.50- Your available balance is USD 5,432.10`,
			nil),
		seed("hsbc.en.ipn_inward", "HSBC", hsbc, income, "IPN inward transfer",
			`Your HSBC Account {{account_id}} was credited with IPN inward transfer for {{currency}} {{amount}} on {{date}} {{time}} from {{counterparty}} with reference {{ref}}. {{*}}`,
			`Your HSBC Account ********1234 was credited with IPN inward transfer for USD 500.00 on 31-08-2025 18:31 from JOHN DOE with reference abc123xyz. For further details, please contact HSBC call centre`,
			nil),
		seed("hsbc.en.ipn_outward", "HSBC", hsbc, expense, "IPN outward transfer",
			`Your HSBC Account {{account_id}} was debited with IPN outward transfer for {{currency}} {{amount}} on {{date}} {{time}} to {{counterparty}} with reference {{ref}}.{{*}}`,
			`Your HSBC Account ********2468 was debited with IPN outward transfer for EGP 1,001.00 on 14-09-2025 17:31 to 8821 with reference f204be32. For further details, please contact HSBC call centre`,
			nil),
		seed("hsbc.en.salary", "HSBC", hsbc, income, "Salary",
			`From HSBC: {{date}} Salary to {{account_id}} {{currency}} {{amount}} Your available balance is {{available_currency}} {{available_amount}}`,
			`From HSBC: 22JUL25 Salary to 123-456***-789 USD 5,000.00+ Your available balance is USD 5,000.00`,
			nil),
		seed("hsbc.en.transfer", "HSBC", hsbc, transfer, "Transfer",
			`From HSBC: {{date}} Transfer from {{account_id}} {{currency}} {{amount}} Your available balance is {{available_currency}} {{available_amount}}`,
			`From HSBC: 07OCT25 Transfer from 123-456***-789 EGP 360.00- Your available balance is EGP 49,647.85`,
			map[string]string{"note": "Transfer"}),

		// CIB
		seed("cib.ar.atm_withdrawal", "CIB", cib, transfer, "ATM withdrawal",
			`تم سحب مبلغ {{currency}} {{amount}} من بطاقة الخصم المباشر المنتهية بـ {{account_id}} من {{note}} في {{date}} {{time}} ، الرصيد المتاح {{available_currency}} {{available_amount}}`,
			`تم سحب مبلغ  EGP 500.00  من بطاقة الخصم المباشر المنتهية بـ **1234 من CIB BRANCH A ATM في 15/10/25 18:12 ، الرصيد المتاح EGP 4500.00`,
			toCash),
		seed("cib.ar.debit_purchase", "CIB", cib, expense, "Debit card purchase",
			`تم خصم مبلغ {{currency}} {{amount}} من بطاقة الخصم المباشر المنتهية بـ {{account_id}} عند {{counterparty:merchant}} في {{date}} {{time}} ، الرصيد المتاح {{available_amount}} {{available_currency}}`,
			`تم خصم مبلغ  EGP 125.50 من بطاقة الخصم المباشر المنتهية بـ **1234 عند Generic Store في 05/07/25 21:45 ، الرصيد المتاح 5432.10 EGP`,
			nil),
		seed("cib.ar.debit_to_credit", "CIB", cib, transfer, "Credit card payment",
			`تم سداد مبلغ {{amount}} {{currency}} فى بطاقتكم الائتمانية المنتهية بـ {{to_account_id}} بتاريخ {{date}}`,
			`تم سداد مبلغ 500.00 جم فى بطاقتكم الائتمانية المنتهية بـ 5678 بتاريخ 05-12-25`,
			nil),
		seed("cib.en.debit_to_credit", "CIB", cib, transfer, "Transfer to another account",
			`Your account ending with {{account_id}} is debited with amount {{currency}} {{amount}}DR On {{date}} with transfer to another account.{{*}}`,
			`Your account ending with ********1234 is debited with amount EGP 750.00DR On 07 DEC 2025 with transfer to  another account.  For more information, please contact 19666.`,
			nil),
		seed("cib.ar.hold_release", "CIB", cib, refund, "Hold release",
			`تم اضافة المعاملة من {{counterparty:merchant}} بقيمة {{amount}} {{currency}} إلى بطاقتك المنتهية بـ {{account_id}} . الرصيد المتاح {{available_currency}} {{available_amount}}`,
			`تم اضافة المعاملة من Ride Service A بقيمة 45.20 EGP إلى بطاقتك المنتهية بـ **1234 . الرصيد المتاح EGP 5432.10`,
			nil),
		seed("cib.ar.refund", "CIB", cib, refund, "Refund",
			`تم اضافة المعاملة الخاصة بـ {{counterparty:merchant}} على بطاقتكم المنتهية بـ {{account_id}} بقيمة {{currency}} {{amount}} بتاريخ {{date}}`,
			`تم اضافة المعاملة الخاصة بـ ONLINE MARKETPLACE STORE XYZ REFUND على بطاقتكم المنتهية بـ 1234 بقيمة EGP 125.00 بتاريخ 27-07-2025`,
			nil),
		seed("cib.ar.ipn_inward", "CIB", cib, income, "IPN inward transfer",
			`يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ {{amount}} {{currency}} إلى حسابك المنتهي بـ {{account_id}} من {{counterparty}} برقم مرجعي {{ref}} بتاريخ {{date}} {{time}} {{*}}`,
			`يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 1000.00 جم إلى حسابك المنتهي بـ ********1234 من JOHN DOE SMITH برقم مرجعي a1b2c3d4 بتاريخ 07-11-2025 16:54 للمزيد، برجاء الاتصال بـ 19666`,
			nil),
		seed("cib.ar.ipn_outward", "CIB", cib, expense, "IPN outward transfer",
			`يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ {{amount}} {{currency}} من حسابك المنتهي بـ {{account_id}} برقم مرجعي {{ref}} بتاريخ {{date}} {{time}} {{*}}`,
			`يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ 350.00 جم من حسابك المنتهي بـ ********1234 برقم مرجعي s5t6u7v8 بتاريخ 08-11-2025 21:51 للمزيد، برجاء الاتصال بـ 19666`,
			nil),
		seed("cib.en.cc_purchase", "CIB", cib, expense, "Credit card purchase",
			`Your credit card ending with{{account_id}} was charged for {{currency}} {{amount}} at {{counterparty:merchant}} on {{date}} at {{time}}. Card available limit is {{available_currency}} {{available_amount}}. {{*}}`,
			`Your credit card ending with#9545 was charged for EGP 235.62 at Uber on 13/05/26  at 10:21. Card available limit is EGP  67766.74. For more details, please visit https://cib.eg/mb`,
			nil),
		seed("cib.en.cc_charge", "CIB", cib, expense, "Credit card charge",
			`Your credit card {{account_id}} was charged for {{currency}} {{amount}} at {{counterparty:merchant}} on {{date}} at {{time}}. Available limit is {{available_amount}}{{*}}`,
			`Your credit card #5678 was charged for EGP 1,728.29 at Generic Store ABC on 25/10/26  at 10:00. Available limit is  78151.20 and the available international limit to use is EGP 135898`,
			nil),
		seed("cib.en.cc_reversal", "CIB", cib, refund, "Credit card reversal",
			`The transaction on your credit card {{account_id}} from {{counterparty:merchant}} with {{currency}} {{amount}} on {{date}} at {{time}} has been refunded.{{*}}`,
			`The transaction on your credit card#5678  from Ride Service  with EGP 247.00 on 21/10/26  at 18:04 has been refunded. Please try again. Thank you`,
			nil),
		seed("cib.ar.cc_refund", "CIB", cib, refund, "Credit card refund",
			`لقد تم رد {{currency}}{{amount}} على بطاقتكم الائتمانية المنتهية بـ# {{account_id}} من {{counterparty:merchant}}> .{{*}}`,
			`لقد تم رد EGP3227.02 على بطاقتكم الائتمانية المنتهية بـ# 5678 من Online Shop>  . يرجى ملاحظة أن هذا المبلغ سيتم إضافته إلى رصيد بطاقتك ولا يتم اعتباره بمثابة دفعة للمديونيات المستحقة لهذا الشهر.`,
			nil),
		ignore("cib.ar.refund_request", "CIB", cib, "Refund request (the refund comes later)",
			`تم استلام الطلب لرد مبلغ {{*}}`,
			`تم استلام الطلب لرد مبلغ من Online Shop بقيمة EGP -320.00 وسيتم اضافته للبطاقة الائتمانية # 5678 بمجرد استلام المبلغ من التاجر. للمزيد من التفاصيل http://cib.eg/mb`),
		ignore("cib.ar.statement", "CIB", cib, "Credit card statement",
			`كشف حساب شهر {{*}}`,
			`كشف حساب شهر أكتوبر 2026  لبطاقتك الائتمانية المنتهية بـ 5678`),
		ignore("cib.ar.auto_payment_reminder", "CIB", cib, "Credit card auto payment reminder",
			`عميلنا العزيز، سيتم خصم {{*}}`,
			`عميلنا العزيز، سيتم خصم 19694.80 جم من حسابكم لسداد بطاقتك المنتهية بـ 5678  في 05/10, في حالة أن يوم السداد يزامن عطلة رسمية سوف يتم الخصم قبل العطلات الرسمية.`),
		ignore("cib.ar.declined_cvv", "CIB", cib, "Declined, wrong CVV",
			`لقد تم رفض المعاملة من {{/.+/}} على بطاقتكم الائتمانية المنتهية بـ{{/\d+/}} بقيمة {{/[\d.,]+/}} {{/\w{3}/}} نظراً لوجود خطأ برمز التحقق المدون خلف البطاقة. يرجى إعادة إدخال رمز التحقق الصحيح.`,
			`لقد تم رفض المعاملة من Vox Cinemas على بطاقتكم الائتمانية المنتهية بـ9545 بقيمة 235.00 EGP نظراً لوجود خطأ برمز التحقق المدون خلف البطاقة. يرجى إعادة إدخال رمز التحقق الصحيح.`),

		// AAIB
		seed("aaib.en.cc_purchase", "AAIB", aaib, expense, "Credit card purchase",
			`Your credit card {{account_id}} was debited by {{amount}}{{currency}} at {{counterparty:merchant}}. Available limit is {{available_amount}}{{*}}`,
			`Your credit card **1432 was debited by 2 111.00EGP at PAYMOBS*Sympl. Available limit is 1 284.40. To install it up to 60 months, call 19555. T&Cs apply.`,
			nil),
		seed("aaib.en.cc_payment", "AAIB", aaib, transfer, "Credit card payment",
			`Credit Card Payment of {{currency}} {{amount}} has been debited from your account {{account_id}} through AAIB Mobile, to your credit card ends with {{to_account_id}}`,
			`Credit Card Payment of EGP 4259 has been debited from your account 1001-01 through AAIB Mobile, to your credit card ends with 1432`,
			nil),
		ignore("aaib.en.cc_payment_credited", "AAIB", aaib, "Credit card payment received (recorded from the payment sms)",
			`Dear Valued customer, your AAIB card {{/\S+/}} has been credited with {{*}}`,
			`Dear Valued customer, your AAIB card **1432 has been credited with 4 259.00EGP on cairo your available balance is 4 266.40.`),
		ignore("aaib.en.cc_declined", "AAIB", aaib, "Declined transaction",
			`Your transaction on credit card {{*}} was declined {{*}}`,
			`Your transaction on credit card **1432 for 65.00 EGP at Uber was declined due to Insufficient balance. Your available limit is 32.7.`),

		// Arab Bank, the body starts with a country code line
		seed("arabbank.ar.salary", "Arab Bank", arabBank, income, "Salary",
			`{{/(?:[A-Z]{2})?/}}قيد راتب بمبلغ {{currency}} {{amount}} لحساب {{account_id}} بتاريخ {{date}}{{*}}`,
			"EG\n قيد راتب بمبلغ USD 1000.00 لحساب 123*4567 بتاريخ 26/10/2026 .",
			nil),
	}
}
