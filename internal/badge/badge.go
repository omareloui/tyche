// Package badge keeps the installed apps' badge in sync with what needs the
// user's attention: sms that made no transaction, transactions to review and
// overdue scheduled ones. Every installed app (a Web Push subscription)
// picks the counts it shows; on a change it gets one silent notification,
// replaced in place, which is also what puts a dot on the icon on Android
// (no Badging API there).
package badge

import (
	"slices"
	"strings"
)

// Source is a count the badge can show.
type Source string

const (
	SourceSMS     Source = "sms"
	SourceReview  Source = "review"
	SourceOverdue Source = "overdue"
)

// Sources are every source, in the order shown.
var Sources = []Source{SourceSMS, SourceReview, SourceOverdue}

// Counts are what needs the user's attention.
type Counts struct {
	SMS     int `json:"sms"`
	Review  int `json:"review"`
	Overdue int `json:"overdue"`
}

// Of is the count of a source.
func (c Counts) Of(s Source) int {
	switch s {
	case SourceSMS:
		return c.SMS
	case SourceReview:
		return c.Review
	case SourceOverdue:
		return c.Overdue
	}
	return 0
}

// Only is c with the counts of the sources not in srcs zeroed.
func (c Counts) Only(srcs []Source) Counts {
	var o Counts
	if slices.Contains(srcs, SourceSMS) {
		o.SMS = c.SMS
	}
	if slices.Contains(srcs, SourceReview) {
		o.Review = c.Review
	}
	if slices.Contains(srcs, SourceOverdue) {
		o.Overdue = c.Overdue
	}
	return o
}

// Total is the badge's number.
func (c Counts) Total() int {
	return c.SMS + c.Review + c.Overdue
}

// URL is the page to open from the notification: the first source with
// something in it.
func (c Counts) URL() string {
	switch {
	case c.SMS > 0:
		return "/sms"
	case c.Review > 0:
		return "/transactions?status=review"
	case c.Overdue > 0:
		return "/transactions?status=scheduled"
	}
	return "/"
}

// ParseSources reads a comma separated list of sources, dropping unknown and
// repeated ones.
func ParseSources(s string) []Source {
	var srcs []Source
	for p := range strings.SplitSeq(s, ",") {
		src := Source(strings.TrimSpace(p))
		if slices.Contains(Sources, src) && !slices.Contains(srcs, src) {
			srcs = append(srcs, src)
		}
	}
	return srcs
}

// JoinSources is the stored form of srcs (ParseSources reads it back).
func JoinSources(srcs []Source) string {
	parts := make([]string, len(srcs))
	for i, s := range srcs {
		parts[i] = string(s)
	}
	return strings.Join(parts, ",")
}
