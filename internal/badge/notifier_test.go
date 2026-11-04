package badge

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/omareloui/tyche/internal/shareddb"
	"github.com/omareloui/tyche/internal/sharedqueries"
	_ "github.com/omareloui/tyche/internal/sqlite"
)

func TestCountsOnly(t *testing.T) {
	c := Counts{SMS: 1, Review: 2, Overdue: 4}
	if got := c.Only(ParseSources("overdue,sms,bogus,sms")).Total(); got != 5 {
		t.Errorf("got %d, want 5", got)
	}
	if got := c.Only(nil).Total(); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
	if got := (Counts{Overdue: 1}).URL(); got != "/transactions?status=scheduled" {
		t.Errorf("url: got %q", got)
	}
}

func TestNotifierPush(t *testing.T) {
	schema, err := filepath.Abs("../../sql")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	t.Chdir(tmp)
	if err := os.Symlink(schema, filepath.Join(tmp, "sql")); err != nil {
		t.Fatal(err)
	}
	db, q, err := shareddb.Open("data/shared.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	var counts Counts
	n := NewNotifier(q, func(context.Context) (Counts, error) { return counts, nil }, Config{PublicKey: "pub", PrivateKey: "priv"})

	type sent struct {
		endpoint string
		p        Payload
	}
	var got []sent
	status := http.StatusCreated
	n.send = func(_ context.Context, payload []byte, s *webpush.Subscription) (int, error) {
		var p Payload
		if err := json.Unmarshal(payload, &p); err != nil {
			t.Fatal(err)
		}
		got = append(got, sent{s.Endpoint, p})
		return status, nil
	}
	sub := func(endpoint string, srcs ...Source) {
		t.Helper()
		err := q.UpsertPushSubscription(ctx, upsertParams(endpoint, "GL_1", JoinSources(srcs)))
		if err != nil {
			t.Fatal(err)
		}
	}
	push := func(c Counts) []sent {
		t.Helper()
		got, counts = nil, c
		if err := n.push(ctx, "GL_1", c); err != nil {
			t.Fatal(err)
		}
		return got
	}

	sub("https://push/a", SourceSMS, SourceReview)
	sub("https://push/b", SourceOverdue)

	// Nothing to show on new subscriptions: no push (Chrome would show its
	// own notification for an empty one).
	if s := push(Counts{}); len(s) != 0 {
		t.Fatalf("empty: sent %+v", s)
	}
	s := push(Counts{SMS: 1, Review: 1})
	if len(s) != 1 || s[0].endpoint != "https://push/a" || s[0].p.Total != 2 || s[0].p.URL != "/sms" {
		t.Fatalf("first counts: sent %+v", s)
	}
	if s := push(Counts{SMS: 1, Review: 1}); len(s) != 0 {
		t.Fatalf("same counts: sent %+v", s)
	}
	// Only b counts overdue ones.
	s = push(Counts{SMS: 1, Review: 1, Overdue: 3})
	if len(s) != 1 || s[0].endpoint != "https://push/b" || s[0].p.Total != 3 || s[0].p.SMS != 0 {
		t.Fatalf("overdue: sent %+v", s)
	}
	// Back to nothing clears the shown badges.
	s = push(Counts{})
	if len(s) != 2 || s[0].p.Total != 0 || s[1].p.Total != 0 {
		t.Fatalf("cleared: sent %+v", s)
	}
	// Subscribing again with the same sources keeps what was sent.
	sub("https://push/a", SourceSMS, SourceReview)
	if s := push(Counts{}); len(s) != 0 {
		t.Fatalf("resubscribed: sent %+v", s)
	}

	// A gone subscription is dropped, the tenant too once it has none.
	n.tenants["GL_1"] = true
	status = http.StatusGone
	push(Counts{SMS: 2, Overdue: 2})
	subs, err := q.ListPushSubscriptions(ctx, "GL_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 0 || n.tenants["GL_1"] {
		t.Fatalf("gone: %d subscriptions left, tenant kept %v", len(subs), n.tenants["GL_1"])
	}
}

func upsertParams(endpoint, tenantID, sources string) sharedqueries.UpsertPushSubscriptionParams {
	return sharedqueries.UpsertPushSubscriptionParams{Endpoint: endpoint, TenantID: tenantID, P256dh: "k", Auth: "a", Sources: sources}
}
