package badge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"slices"
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

// maxPayload is how big a push's payload gets, under the push services'
// 4 KB with room for the encryption.
const maxPayload = 3500

// Payload is what a push sends the service worker (sw.js).
type Payload struct {
	Counts
	Total int    `json:"total"`
	URL   string `json:"url"`
	// More is how many weren't sent as items, shown as one more
	// notification titled Title.
	More  int    `json:"more"`
	Title string `json:"title"`
}

// payload is the push of c, its items cut to fit maxPayload.
func payload(c Counts) ([]byte, error) {
	for {
		total := c.Total()
		b, err := json.Marshal(Payload{Counts: c, Total: total, URL: c.URL(), More: total - len(c.Items), Title: c.Title()})
		if err != nil || len(b) <= maxPayload || len(c.Items) == 0 {
			return b, err
		}
		c.Items = c.Items[:len(c.Items)-1]
	}
}

// signature tells apart what a push shows, kept in last_sent: 0 for
// nothing, else a hash of the total and the items.
func signature(c Counts) int64 {
	total := c.Total()
	if total == 0 {
		return 0
	}
	tags := make([]string, len(c.Items))
	for i, it := range c.Items {
		tags[i] = it.Tag
	}
	slices.Sort(tags)
	h := fnv.New64a()
	fmt.Fprintf(h, "%d|%s", total, strings.Join(tags, ","))
	if sig := int64(h.Sum64() & math.MaxInt64); sig != 0 {
		return sig
	}
	return 1
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
// current counts. Its dismissed notifications are shown again repostHours
// later while unhandled, never when 0.
func (n *Notifier) Subscribe(ctx context.Context, tenantID string, sub webpush.Subscription, srcs []Source, repostHours int) error {
	err := n.q.UpsertPushSubscription(ctx, sharedqueries.UpsertPushSubscriptionParams{
		Endpoint:    sub.Endpoint,
		TenantID:    tenantID,
		P256dh:      sub.Keys.P256dh,
		Auth:        sub.Keys.Auth,
		Sources:     JoinSources(srcs),
		RepostHours: int64(repostHours),
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

// Dismissed notes that the user dismissed one of the subscription's
// notifications: they're pushed a repost after its repost hours (RunReposts),
// the service worker showing again the dismissed ones still unhandled. The
// endpoint, known only to the browser and us, is what authorizes it.
func (n *Notifier) Dismissed(ctx context.Context, endpoint string) error {
	return n.q.SetPushRepost(ctx, sharedqueries.SetPushRepostParams{Now: time.Now().Unix(), Endpoint: endpoint})
}

// repostPayload asks the service worker to show the dismissed notifications
// again, from what it was last sent: no tenant database is opened for it.
var repostPayload = []byte(`{"repost":true}`)

// RunReposts pushes the due reposts every minute until ctx is done.
func (n *Notifier) RunReposts(ctx context.Context) {
	if !n.Enabled() {
		return
	}
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := n.repost(ctx); err != nil {
			logger.Get().Error("reposting the badge's notifications", zap.Error(err))
		}
	}
}

// repost pushes the subscriptions whose repost is due.
func (n *Notifier) repost(ctx context.Context) error {
	n.sendMu.Lock()
	defer n.sendMu.Unlock()
	subs, err := n.q.ListDuePushReposts(ctx, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("listing the due reposts: %w", err)
	}
	var errs []error
	for _, s := range subs {
		errs = append(errs, n.q.ClearPushRepost(ctx, s.Endpoint))
		if s.LastSent <= 0 {
			// Nothing left to show.
			continue
		}
		ctx, cancel := context.WithTimeout(ctx, sendTimeout)
		status, err := n.send(ctx, repostPayload, &webpush.Subscription{
			Endpoint: s.Endpoint,
			Keys:     webpush.Keys{P256dh: s.P256dh, Auth: s.Auth},
		})
		cancel()
		switch {
		case status == http.StatusNotFound || status == http.StatusGone:
			errs = append(errs, n.q.DeleteGonePushSubscription(ctx, s.Endpoint), n.reloadTenant(ctx, s.TenantID))
		case err != nil:
			errs = append(errs, err)
		case status >= 300:
			errs = append(errs, fmt.Errorf("push service answered %d", status))
		}
	}
	return errors.Join(errs...)
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

// push sends c to each subscription of the tenant whose counts or items
// changed. A total of 0 is only sent to clear a badge shown, not to a new
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
		sig := signature(mine)
		if sig == s.LastSent || (sig == 0 && s.LastSent <= 0) {
			if s.LastSent < 0 {
				errs = append(errs, n.setSent(ctx, s.Endpoint, sig))
			}
			continue
		}
		body, err := payload(mine)
		if err != nil {
			return err
		}
		status, err := n.send(ctx, body, &webpush.Subscription{
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
			errs = append(errs, n.setSent(ctx, s.Endpoint, sig))
		}
	}
	if gone {
		errs = append(errs, n.reloadTenant(ctx, tenantID))
	}
	return errors.Join(errs...)
}

func (n *Notifier) setSent(ctx context.Context, endpoint string, sig int64) error {
	return n.q.SetPushSubscriptionSent(ctx, sharedqueries.SetPushSubscriptionSentParams{LastSent: sig, Endpoint: endpoint})
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
