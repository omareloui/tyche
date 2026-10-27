package models

import "time"

type SMSStatus string

const (
	// SMSStatusParsed means a transaction got created from the sms.
	SMSStatusParsed SMSStatus = "parsed"
	// SMSStatusUnparsed means no parser could read the sms.
	SMSStatusUnparsed SMSStatus = "unparsed"
	// SMSStatusUnmatched means the sms got parsed but no account matched it.
	SMSStatusUnmatched SMSStatus = "unmatched"
	// SMSStatusFailed means creating the transaction failed, e.g. no fx rate.
	SMSStatusFailed SMSStatus = "failed"
	// SMSStatusIgnored means the user dismissed the sms.
	SMSStatusIgnored SMSStatus = "ignored"
	// SMSStatusDuplicate is never stored, it's returned when the sms was
	// already received.
	SMSStatusDuplicate SMSStatus = "duplicate"
)

type SMSMessage struct {
	ID            string
	Sender        string
	Body          string
	ReceivedAt    time.Time
	Hash          string
	Status        SMSStatus
	Error         string
	TransactionID string
	DeviceID      string
	CreatedAt     time.Time
}
