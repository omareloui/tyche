package badge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/sharedqueries"
	"go.uber.org/zap"
)

const (
	sendTimeout = 30 * time.Second
	// pushTTL is how long the push service keeps a notification for an
	// offline device, a newer one replacing it on the device anyway.
	pushTTL = 24 * 60 * 60
	// pushTopic collapses the pending notifications of a subscription into
	// the latest one.
	pushTopic = "attention"
)

// Counter counts what needs the attention of the tenant in the context.
type Counter func(ctx context.Context) (Counts, error)

// Config are the VAPID keys signing the pushes.
type Config struct {
	PublicKey  string
	PrivateKey string
	// Subject is a mailto: or https: contact for the push services.
	Subject string
}

// Payload is what a push sends the service worker (sw.js).
type Payload struct {
	Counts
	Total int    `json:"total"`
	URL   string `json:"url"`
}

// sendFunc sends a push, answering the push service's status.
type sendFunc func(ctx context.Context, payload []byte, sub *webpush.Subscription) (int, error)

// Notifier pushes the badge's counts to the installed apps of a tenant when
// they change. Counting needs the tenant's database (unlocked, when it's
// encrypted), so it's only done along a request of the tenant (AfterRequest):
// overdue transactions only show once the database is opened that day.
type Notifier struct {
	q     *sharedqueries.Queries
	count Counter
	cfg   Config
	send  sendFunc

	mu sync.Mutex
	// tenants are the tenants with a subscription, so the others are
	// never counted.
	tenants map[string]bool
	// counted is the day each tenant was last counted.
	counted map[string]string

	// sendMu serializes the refreshes, keeping each subscription's
	// last_sent right and the latest counts pushed last.
	sendMu sync.Mutex
}

func NewNotifier(q *sharedqueries.Queries, count Counter, cfg Config) *Notifier {
	n := &Notifier{
		q:       q,
		count:   count,
		cfg:     cfg,
		tenants: make(map[string]bool),
		counted: make(map[string]string),
	}
	n.send = n.webpush
	return n
}

// Enabled is whether the VAPID keys are set: without them nothing can be
// pushed and the badge settings aren't offered.
func (n *Notifier) Enabled() bool {
	return n != nil && n.cfg.PublicKey != "" && n.cfg.PrivateKey != ""
}

// PublicKey is the VAPID key the browsers subscribe with.
func (n *Notifier) PublicKey() string {
	return n.cfg.PublicKey
}

// Load reads which tenants have subscriptions.
func (n *Notifier) Load(ctx context.Context) error {
	ids, err := n.q.ListPushTenants(ctx)
	if err != nil {
		return fmt.Errorf("listing the push tenants: %w", err)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, id := range ids {
		n.tenants[id] = true
	}
	return nil
}

// Subscribe adds (or updates) a browser's subscription, then pushes it the
// current counts.
func (n *Notifier) Subscribe(ctx context.Context, tenantID string, sub webpush.Subscription, srcs []Source) error {
	err := n.q.UpsertPushSubscription(ctx, sharedqueries.UpsertPushSubscriptionParams{
		Endpoint: sub.Endpoint,
		TenantID: tenantID,
		P256dh:   sub.Keys.P256dh,
		Auth:     sub.Keys.Auth,
		Sources:  JoinSources(srcs),
	})
	if err != nil {
		return fmt.Errorf("saving the push subscription: %w", err)
	}
	n.mu.Lock()
	n.tenants[tenantID] = true
	n.mu.Unlock()
	n.Refresh(ctx, tenantID)
	return nil
}

// Unsubscribe drops a browser's subscription.
func (n *Notifier) Unsubscribe(ctx context.Context, tenantID, endpoint string) error {
	err := n.q.DeletePushSubscription(ctx, sharedqueries.DeletePushSubscriptionParams{Endpoint: endpoint, TenantID: tenantID})
	if err != nil {
		return fmt.Errorf("deleting the push subscription: %w", err)
	}
	return n.reloadTenant(ctx, tenantID)
}

// Forget drops every subscription of a tenant, e.g. its account deleted.
func (n *Notifier) Forget(ctx context.Context, tenantID string) error {
	if n == nil {
		return nil
	}
	if err := n.q.DeleteTenantPushSubscriptions(ctx, tenantID); err != nil {
		return fmt.Errorf("deleting the push subscriptions: %w", err)
	}
	n.mu.Lock()
	delete(n.tenants, tenantID)
	delete(n.counted, tenantID)
	n.mu.Unlock()
	return nil
}

// AfterRequest counts again after a request of the tenant: one that may have
// changed something (write), or its first one of the day (overdue ones).
func (n *Notifier) AfterRequest(ctx context.Context, tenantID string, write bool) {
	if !n.Enabled() {
		return
	}
	today := time.Now().UTC().Format(time.DateOnly)
	n.mu.Lock()
	due := n.tenants[tenantID] && (write || n.counted[tenantID] != today)
	n.mu.Unlock()
	if due {
		n.Refresh(ctx, tenantID)
	}
}

// Refresh counts with the tenant's database in ctx and pushes the counts to
// the subscriptions whose total changed, in the background: one at a time,
// so the last push has the latest counts.
func (n *Notifier) Refresh(ctx context.Context, tenantID string) {
	if !n.Enabled() {
		return
	}
	l := logger.FromCtx(ctx)
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendTimeout)
		defer cancel()
		n.sendMu.Lock()
		defer n.sendMu.Unlock()

		c, err := n.count(ctx)
		if err != nil {
			// E.g. locked: counted again on its next request.
			l.Debug("counting the badge", zap.Error(err))
			return
		}
		n.mu.Lock()
		n.counted[tenantID] = time.Now().UTC().Format(time.DateOnly)
		n.mu.Unlock()
		if err := n.push(ctx, tenantID, c); err != nil {
			l.Error("pushing the badge", zap.Error(err))
		}
	}()
}

// push sends c to each subscription of the tenant whose total changed. A
// total of 0 is only sent to clear a badge shown, not to a new
// subscription: a push showing nothing makes Chrome show its own
// notification. Called holding sendMu, keeping each last_sent right.
func (n *Notifier) push(ctx context.Context, tenantID string, c Counts) error {
	subs, err := n.q.ListPushSubscriptions(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("listing the push subscriptions: %w", err)
	}
	var errs []error
	gone := false
	for _, s := range subs {
		mine := c.Only(ParseSources(s.Sources))
		total := mine.Total()
		if int64(total) == s.LastSent || (total == 0 && s.LastSent <= 0) {
			if s.LastSent < 0 {
				errs = append(errs, n.setSent(ctx, s.Endpoint, total))
			}
			continue
		}
		payload, err := json.Marshal(Payload{Counts: mine, Total: total, URL: mine.URL()})
		if err != nil {
			return err
		}
		status, err := n.send(ctx, payload, &webpush.Subscription{
			Endpoint: s.Endpoint,
			Keys:     webpush.Keys{P256dh: s.P256dh, Auth: s.Auth},
		})
		switch {
		case status == http.StatusNotFound || status == http.StatusGone:
			// Unsubscribed or expired.
			errs = append(errs, n.q.DeleteGonePushSubscription(ctx, s.Endpoint))
			gone = true
		case err != nil:
			errs = append(errs, err)
		case status >= 300:
			errs = append(errs, fmt.Errorf("push service answered %d", status))
		default:
			errs = append(errs, n.setSent(ctx, s.Endpoint, total))
		}
	}
	if gone {
		errs = append(errs, n.reloadTenant(ctx, tenantID))
	}
	return errors.Join(errs...)
}

func (n *Notifier) setSent(ctx context.Context, endpoint string, total int) error {
	return n.q.SetPushSubscriptionSent(ctx, sharedqueries.SetPushSubscriptionSentParams{LastSent: int64(total), Endpoint: endpoint})
}

// reloadTenant drops the tenant from the subscribed ones once it has none.
func (n *Notifier) reloadTenant(ctx context.Context, tenantID string) error {
	subs, err := n.q.ListPushSubscriptions(ctx, tenantID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(subs) == 0 {
		delete(n.tenants, tenantID)
		delete(n.counted, tenantID)
	}
	return nil
}

func (n *Notifier) webpush(ctx context.Context, payload []byte, sub *webpush.Subscription) (int, error) {
	res, err := webpush.SendNotificationWithContext(ctx, payload, sub, &webpush.Options{
		Subscriber:      strings.TrimPrefix(n.cfg.Subject, "mailto:"),
		TTL:             pushTTL,
		Topic:           pushTopic,
		Urgency:         webpush.UrgencyNormal,
		VAPIDPublicKey:  n.cfg.PublicKey,
		VAPIDPrivateKey: n.cfg.PrivateKey,
	})
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode, nil
}
