package router_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestCounterpartyRatingLocationsHTTP(t *testing.T) {
	repo, ctx, usr := setupTenant(t)

	egp, err := repo.GetCurrencyByString(ctx, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	usr.Email, usr.DefaultCurrency = "e2e@example.com", *egp
	if err := repo.CreateUser(ctx, usr); err != nil {
		t.Fatal(err)
	}
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}

	kinds, err := repo.GetAccountKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var bankKind models.AccountKind
	for _, k := range kinds {
		if k.Enum == models.AccKindBankAcc {
			bankKind = k
		}
	}
	bank := &models.Account{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZA", Name: "Bank", Amount: 1000, Currency: *egp, Kind: bankKind}
	if err := repo.CreateAccount(ctx, bank); err != nil {
		t.Fatal(err)
	}
	cafe := &models.Counterparty{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZC", Name: "Cafe", IsMerchant: true}
	if err := repo.CreateCounterparty(ctx, cafe); err != nil {
		t.Fatal(err)
	}
	tx := &models.Transaction{
		ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZD", Account: *bank, Amount: 120, Kind: models.ExpenseTxKind,
		Description: "Flat white", Counterparty: cafe, ScheduledTo: time.Now(), FulfilledAt: time.Now(),
	}
	if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t))))
	t.Cleanup(srv.Close)

	do := func(method, path string, form url.Values) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}

	edit := url.Values{
		"name": {"Cafe"}, "is_merchant": {"on"}, "rating": {"4"}, "note": {"Good wifi"},
		"location_id": {"", ""}, "location_label": {"Zamalek", "Maadi"},
		"location_address": {"26 July St", ""}, "location_maps_url": {"", "javascript:alert(1)"},
	}

	t.Run("an invalid maps link answers the form with its error", func(t *testing.T) {
		res, body := do("PUT", "/counterparties/"+cafe.ID, edit)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "valid URL") {
			t.Fatalf("status %d, body without the url error: %s", res.StatusCode, body)
		}
	})

	t.Run("saving keeps rating, note and locations", func(t *testing.T) {
		edit["location_maps_url"] = []string{"", "https://maps.app.goo.gl/x"}
		res, body := do("PUT", "/counterparties/"+cafe.ID, edit)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		for _, want := range []string{"Good wifi", "2 locations", `data-rating="4"`} {
			if !strings.Contains(body, want) {
				t.Errorf("card misses %q", want)
			}
		}
	})

	t.Run("rating from the card", func(t *testing.T) {
		res, body := do("PUT", "/counterparties/"+cafe.ID+"/rating", url.Values{"rating": {"2"}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, `data-rating="2"`) {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		got, err := repo.GetCounterparty(ctx, cafe.ID)
		if err != nil || got.Rating != 2 || len(got.Locations) != 2 {
			t.Errorf("stored = %+v (%v), want rating 2 and its 2 locations", got, err)
		}
	})

	t.Run("details", func(t *testing.T) {
		res, body := do("GET", "/counterparties/"+cafe.ID+"/details", nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		for _, want := range []string{"Good wifi", "Zamalek", "https://maps.app.goo.gl/x", "maps/search/?api=1&amp;query=26+July+St", "Flat white", `hx-vals="{&#34;rating&#34;: &#34;0&#34;, &#34;from&#34;: &#34;details&#34;}"`, "Add location"} {
			if !strings.Contains(body, want) {
				t.Errorf("details miss %q", want)
			}
		}
	})

	t.Run("rating from the details only updates the card", func(t *testing.T) {
		res, body := do("PUT", "/counterparties/"+cafe.ID+"/rating", url.Values{"rating": {"5"}, "from": {"details"}})
		if res.StatusCode != http.StatusOK || res.Header.Get("HX-Reswap") != "none" || !strings.Contains(body, `hx-swap-oob="innerHTML"`) {
			t.Fatalf("status %d, reswap %q: %s", res.StatusCode, res.Header.Get("HX-Reswap"), body)
		}
		if got, err := repo.GetCounterparty(ctx, cafe.ID); err != nil || got.Rating != 5 {
			t.Errorf("stored = %+v (%v), want rating 5", got, err)
		}
	})

	t.Run("adding a location from the details", func(t *testing.T) {
		res, body := do("POST", "/counterparties/"+cafe.ID+"/locations", url.Values{"label": {" "}, "maps_url": {"nope"}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "This field is required") || !strings.Contains(body, "valid URL") {
			t.Fatalf("status %d, missing errors: %s", res.StatusCode, body)
		}

		res, body = do("POST", "/counterparties/"+cafe.ID+"/locations", url.Values{"label": {"Heliopolis"}, "address": {"Korba"}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Heliopolis") || !strings.Contains(body, "3 locations") {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		got, err := repo.GetCounterparty(ctx, cafe.ID)
		if err != nil {
			t.Fatal(err)
		}
		if labels := locationLabels(got.Locations); strings.Join(labels, ",") != "Zamalek,Maadi,Heliopolis" {
			t.Errorf("locations = %q, want Heliopolis last", labels)
		}
	})

	t.Run("a person can't be rated", func(t *testing.T) {
		person := &models.Counterparty{ID: "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZE", Name: "Ahmed"}
		if err := repo.CreateCounterparty(ctx, person); err != nil {
			t.Fatal(err)
		}
		res, _ := do("PUT", "/counterparties/"+person.ID+"/rating", url.Values{"rating": {"3"}})
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("status %d, want 422", res.StatusCode)
		}
	})

	t.Run("labels take the casing in use and filter the page", func(t *testing.T) {
		person, err := repo.GetCounterparty(ctx, "01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZE")
		if err != nil {
			t.Fatal(err)
		}
		person.Labels = []string{"Family"}
		if err := repo.UpdateCounterparty(ctx, person); err != nil {
			t.Fatal(err)
		}

		res, body := do("PUT", "/counterparties/"+cafe.ID, url.Values{
			"name": {"Cafe"}, "is_merchant": {"on"}, "labels": {"family, Cafe ,cafe"},
			"location_label": {"Zamalek", "Maadi", "Heliopolis"},
		})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, `data-labels="family|cafe"`) {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		got, err := repo.GetCounterparty(ctx, cafe.ID)
		if err != nil || strings.Join(got.Labels, ",") != "Family,Cafe" {
			t.Errorf("stored labels = %q (%v), want Family,Cafe", got.Labels, err)
		}

		res, body = do("GET", "/counterparties", nil)
		for _, want := range []string{`aria-label="Filter by label"`, `data-label-filter="cafe"`, `data-label-filter="family"`} {
			if res.StatusCode != http.StatusOK || !strings.Contains(body, want) {
				t.Errorf("page misses %q", want)
			}
		}

		_, body = do("GET", "/counterparties/"+cafe.ID+"/edit", nil)
		if !strings.Contains(body, `data-tag="Online store"`) || !strings.Contains(body, `data-tag="Family"`) {
			t.Error("edit form doesn't suggest the labels in use and the default ones")
		}
	})

	t.Run("editing labels from the details", func(t *testing.T) {
		_, body := do("GET", "/counterparties/"+cafe.ID+"/details", nil)
		if !strings.Contains(body, `id="cp-labels-`+cafe.ID+`"`) || !strings.Contains(body, "Add the Online store label") {
			t.Fatal("details miss the labels editor")
		}

		res, body := do("PUT", "/counterparties/"+cafe.ID+"/labels", url.Values{"labels": {"cafe, Brunch"}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, `hx-swap-oob="innerHTML"`) || !strings.Contains(body, `data-labels="cafe|brunch"`) {
			t.Fatalf("status %d: %s", res.StatusCode, body)
		}
		got, err := repo.GetCounterparty(ctx, cafe.ID)
		if err != nil || strings.Join(got.Labels, ",") != "Cafe,Brunch" || len(got.Locations) != 3 {
			t.Errorf("stored = %q, %d locations (%v), want Cafe,Brunch and the locations kept", got.Labels, len(got.Locations), err)
		}

		res, body = do("PUT", "/counterparties/"+cafe.ID+"/labels", url.Values{"labels": {"Cafe, Brunch"}, "add": {"Late night"}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Remove the Late night label") {
			t.Fatalf("status %d, typed label not added: %s", res.StatusCode, body)
		}

		// Removing one, as its chip sends the others.
		res, body = do("PUT", "/counterparties/"+cafe.ID+"/labels", url.Values{"labels": {"Cafe, Late night"}})
		if res.StatusCode != http.StatusOK || strings.Contains(body, "Remove the Brunch label") {
			t.Fatalf("status %d, Brunch not removed: %s", res.StatusCode, body)
		}
		if got, err := repo.GetCounterparty(ctx, cafe.ID); err != nil || strings.Join(got.Labels, ",") != "Cafe,Late night" {
			t.Fatalf("stored = %q (%v), want Cafe,Late night", got.Labels, err)
		}
		do("PUT", "/counterparties/"+cafe.ID+"/labels", url.Values{"labels": {"Cafe, Brunch, Late night"}})

		res, body = do("PUT", "/counterparties/"+cafe.ID+"/labels", url.Values{"labels": {"Cafe"}, "add": {strings.Repeat("x", 41)}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "form-error-message") {
			t.Errorf("status %d, too long label not rejected: %s", res.StatusCode, body)
		}
		if got, err := repo.GetCounterparty(ctx, cafe.ID); err != nil || strings.Join(got.Labels, ",") != "Cafe,Brunch,Late night" {
			t.Errorf("rejected change saved: %q (%v)", got.Labels, err)
		}
	})

	t.Run("page groups merchants and others", func(t *testing.T) {
		res, body := do("GET", "/counterparties", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Filter by rating") {
			t.Fatalf("status %d, no rating bar", res.StatusCode)
		}
		merchants, others := strings.Index(body, `id="counterparties-merchants"`), strings.Index(body, `id="counterparties-others"`)
		cafeAt, ahmedAt := strings.Index(body, `id="counterparty-`+cafe.ID+`"`), strings.Index(body, `id="counterparty-01K8KQZ4N1XQ5WJ5M3Q6G7Y8ZE"`)
		if merchants < 0 || others < 0 || !(merchants < cafeAt && cafeAt < others && others < ahmedAt) {
			t.Errorf("cards not in their sections: merchants %d, cafe %d, others %d, ahmed %d", merchants, cafeAt, others, ahmedAt)
		}
	})

	t.Run("no longer a merchant moves the card to the others", func(t *testing.T) {
		res, body := do("PUT", "/counterparties/"+cafe.ID, url.Values{"name": {"Cafe"}, "note": {"Good wifi"}})
		if res.StatusCode != http.StatusOK || !strings.Contains(body, `id="counterparties-others" hx-swap-oob="afterbegin"`) {
			t.Fatalf("status %d, card not moved: %s", res.StatusCode, body)
		}
	})
}
