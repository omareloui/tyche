package smsguess

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/gemini"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/smsparser"
)

func guessOf(t models.SMSTemplate) *models.SMSTemplateGuess {
	return &models.SMSTemplateGuess{Name: t.Name, Bank: t.Bank, Action: t.Action, TxKind: t.TxKind, Body: t.Body, Fixed: t.Fixed}
}

func TestVerifySeeds(t *testing.T) {
	for _, s := range smsparser.Seeds() {
		if err := Verify(guessOf(s), s.Sample); err != nil {
			t.Errorf("seed %s: %v", s.SeedKey, err)
		}
	}
}

func TestVerify(t *testing.T) {
	const sms = "Card 1234 charged EGP 50.00 at SHOP. Balance EGP 900"
	cases := map[string]struct {
		g    models.SMSTemplateGuess
		want string
	}{
		"reads":   {g: models.SMSTemplateGuess{Action: "transaction", TxKind: models.ExpenseTxKind, Body: "Card {{account_id}} charged {{currency}} {{amount}} at {{counterparty:merchant}}. {{*}}"}},
		"ignore":  {g: models.SMSTemplateGuess{Action: "ignore", Body: "Card {{/\\d+/}} charged {{*}}"}},
		"empty":   {g: models.SMSTemplateGuess{Action: "transaction", TxKind: models.ExpenseTxKind}, want: "empty"},
		"no kind": {g: models.SMSTemplateGuess{Action: "transaction", Body: "{{*}}"}, want: "tx_kind"},
		"partial match": {g: models.SMSTemplateGuess{Action: "transaction", TxKind: models.ExpenseTxKind, Body: "Card {{account_id}} charged {{currency}} {{amount}}"},
			want: "doesn't read"},
		"bad syntax": {g: models.SMSTemplateGuess{Action: "transaction", TxKind: models.ExpenseTxKind, Body: "Card {{nope}} {{*}}"}, want: "doesn't read"},
		"no amount":  {g: models.SMSTemplateGuess{Action: "transaction", TxKind: models.ExpenseTxKind, Body: "Card {{account_id}} {{*}}"}, want: "amount"},
		"fixed account": {g: models.SMSTemplateGuess{Action: "transaction", TxKind: models.ExpenseTxKind, Body: "Card {{/\\d+/}} charged {{currency}} {{amount}} {{*}}",
			Fixed: map[string]string{"account_id": "Bank"}}},
		"ignore not matching": {g: models.SMSTemplateGuess{Action: "ignore", Body: "OTP {{*}}"}, want: "doesn't match"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := Verify(&tc.g, sms)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

type stubGuesser struct {
	answers []*models.SMSTemplateGuess
	calls   int
}

func (s *stubGuesser) Model() string { return "stub" }

func (s *stubGuesser) Guess(ctx context.Context, in In) (*models.SMSTemplateGuess, gemini.Usage, error) {
	s.calls++
	return s.answers[s.calls-1], gemini.Usage{InputTokens: 10, OutputTokens: 5}, nil
}

func TestGuessVerified(t *testing.T) {
	const sms = "Card 1234 charged EGP 50.00"
	bad := &models.SMSTemplateGuess{Action: "transaction", TxKind: models.ExpenseTxKind, Body: "nope"}
	good := &models.SMSTemplateGuess{Action: "transaction", TxKind: models.ExpenseTxKind, Body: "Card {{account_id}} charged {{currency}} {{amount}}"}

	g := &stubGuesser{answers: []*models.SMSTemplateGuess{bad, good}}
	got, usage, err := GuessVerified(context.Background(), g, In{Body: sms})
	if err != nil || got != good {
		t.Fatalf("got %+v, %v", got, err)
	}
	if usage.InputTokens != 20 || usage.OutputTokens != 10 {
		t.Errorf("usage = %+v, want both calls", usage)
	}

	g = &stubGuesser{answers: []*models.SMSTemplateGuess{bad, bad}}
	if _, _, err := GuessVerified(context.Background(), g, In{Body: sms}); !errors.Is(err, ErrNoGuess) {
		t.Errorf("err = %v", err)
	}
}

func TestPickExamples(t *testing.T) {
	seeds := smsparser.Seeds()
	sender := seeds[len(seeds)-1].Senders[0]
	ex := PickExamples(sender, seeds)
	if len(ex) != MaxExamples {
		t.Fatalf("examples = %d", len(ex))
	}
	if !ex[0].AppliesTo(sender) {
		t.Errorf("the sender's templates come first, got %s", ex[0].SeedKey)
	}
	kinds := map[models.TxKind]bool{}
	for _, e := range ex {
		kinds[e.TxKind] = true
	}
	if len(kinds) < 2 {
		t.Errorf("kinds = %v, want variety", kinds)
	}
}

func TestPrompt(t *testing.T) {
	p := Prompt(In{Sender: "Bank", Body: "the sms", Examples: smsparser.Seeds()[:1],
		Previous: &models.SMSTemplateGuess{Body: "old body"}, PreviousErr: errors.New("why it failed")})
	for _, want := range []string{"{{counterparty:merchant}}", "available_amount", smsparser.Seeds()[0].Sample, "the sms", "old body", "why it failed"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}
