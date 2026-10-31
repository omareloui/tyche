package router_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

func TestSMSTemplates(t *testing.T) {
	repo, ctx, usr := setupTenant(t)
	t.Setenv("ADMIN_TENANT_IDS", models.TenantID(usr.OAuthProvider, usr.OAuthID))
	shared := sharedTemplates(t)

	srv := httptest.NewServer(router.New(handler.New(repo, valid.New(), sanitizer.New(), nil, shared)))
	defer srv.Close()
	pair, err := jwt.NewPair(usr)
	if err != nil {
		t.Fatal(err)
	}

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

	t.Run("lists the seeded templates", func(t *testing.T) {
		res, body := do("GET", "/sms/templates", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Credit card purchase") {
			t.Fatalf("got %d", res.StatusCode)
		}
		if strings.Contains(body, "Broken") {
			t.Error("a seeded template is broken")
		}
	})

	cibCharge := "Your credit card ending with#9545 was charged for EGP 235.62 at Uber on 13/05/26  at 10:21. Card available limit is EGP  67766.74. For more details, please visit https://cib.eg/mb"

	t.Run("sandbox reads with the shared templates", func(t *testing.T) {
		_, body := do("POST", "/sms/templates/sandbox", url.Values{"sender": {"CIB"}, "body": {cibCharge}})
		for _, want := range []string{"Read", "Credit card purchase", "235.62", "Uber (merchant)", "couldn&#39;t match the sms to an account"} {
			if !strings.Contains(body, want) {
				t.Errorf("sandbox result is missing %q", want)
			}
		}
	})

	t.Run("preview shows compile errors and the fields", func(t *testing.T) {
		_, body := do("POST", "/sms/templates/preview", url.Values{"body": {"Paid {{nope}}"}})
		if !strings.Contains(body, "unknown field") {
			t.Errorf("no compile error in %s", body)
		}
		_, body = do("POST", "/sms/templates/preview", url.Values{"body": {"Paid {{amount}} at {{counterparty}}"}, "sample": {"Paid 10 at Shop"}, "action": {"transaction"}, "tx_kind": {"Expense"}})
		if !strings.Contains(body, "The sample matches") || !strings.Contains(body, "Shop") {
			t.Errorf("no match in %s", body)
		}
	})

	t.Run("rejects a template that doesn't read its sample", func(t *testing.T) {
		res, body := do("POST", "/sms/templates", url.Values{
			"scope": {"mine"}, "name": {"x"}, "action": {"transaction"}, "tx_kind": {"Expense"},
			"body": {"Paid {{amount}}"}, "sample": {"Paid at Shop"},
		})
		if res.Header.Get("HX-Redirect") != "" || !strings.Contains(body, "doesn&#39;t read it") {
			t.Errorf("expected the sample error, got %d %s", res.StatusCode, res.Header.Get("HX-Redirect"))
		}
	})

	for _, scope := range []string{"mine", "shared"} {
		t.Run("creates, edits and deletes "+scope+" templates", func(t *testing.T) {
			name := "Test " + scope
			res, _ := do("POST", "/sms/templates", url.Values{
				"scope": {scope}, "name": {name}, "bank": {"Test"}, "senders": {"TEST, 123"},
				"action": {"ignore"}, "body": {"Your OTP is {{/\\d+/}}"}, "sample": {"Your OTP is 1234"}, "is_enabled": {"on"},
			})
			if res.Header.Get("HX-Redirect") != "/sms/templates" {
				t.Fatalf("not created: %d", res.StatusCode)
			}

			var id string
			if scope == "shared" {
				for _, tm := range shared.List() {
					if tm.Name == name {
						id = tm.ID
					}
				}
			} else {
				mine, _ := repo.GetSMSTemplates(ctx)
				for _, tm := range mine {
					if tm.Name == name {
						id = tm.ID
						if len(tm.Senders) != 2 || tm.Action != models.SMSTemplateIgnore || !tm.IsEnabled {
							t.Errorf("unexpected template %+v", tm)
						}
					}
				}
			}
			if id == "" {
				t.Fatal("template not found")
			}

			if res, _ := do("GET", "/sms/templates/"+scope+"/"+id+"/edit", nil); res.StatusCode != http.StatusOK {
				t.Errorf("edit page: %d", res.StatusCode)
			}

			_, body := do("POST", "/sms/templates/sandbox", url.Values{"sender": {"test"}, "body": {"Your OTP is 98765"}})
			if !strings.Contains(body, "Ignored") || !strings.Contains(body, name) {
				t.Errorf("sandbox didn't use the new template")
			}

			res, _ = do("PUT", "/sms/templates/"+scope+"/"+id, url.Values{
				"name": {name + " edited"}, "action": {"ignore"}, "body": {"Your OTP is {{*}}"},
			})
			if res.Header.Get("HX-Redirect") == "" {
				t.Errorf("not updated: %d", res.StatusCode)
			}

			if res, _ := do("DELETE", "/sms/templates/"+scope+"/"+id, nil); res.StatusCode != http.StatusOK {
				t.Errorf("delete: %d", res.StatusCode)
			}
			if res, _ := do("GET", "/sms/templates/"+scope+"/"+id+"/edit", nil); res.StatusCode != http.StatusNotFound {
				t.Errorf("deleted template's edit page: %d", res.StatusCode)
			}
		})
	}

	t.Run("non admins only write their own templates", func(t *testing.T) {
		t.Setenv("ADMIN_TENANT_IDS", "")
		cibID := ""
		for _, tm := range shared.List() {
			if tm.SeedKey == "cib.en.cc_purchase" {
				cibID = tm.ID
			}
		}

		res, body := do("GET", "/sms/templates", nil)
		if res.StatusCode != http.StatusOK || !strings.Contains(body, "Credit card purchase") {
			t.Fatalf("list: %d", res.StatusCode)
		}
		if strings.Contains(body, "/sms/templates/shared/"+cibID+"/edit") || !strings.Contains(body, "copy="+cibID) {
			t.Error("shared templates should have an override link instead of edit")
		}

		res, _ = do("POST", "/sms/templates", url.Values{
			"scope": {"shared"}, "name": {"nope"}, "action": {"ignore"}, "body": {"x {{*}}"},
		})
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("create shared: %d", res.StatusCode)
		}
		for _, m := range []string{"GET", "PUT", "DELETE"} {
			path := "/sms/templates/shared/" + cibID
			if m == "GET" {
				path += "/edit"
			}
			if res, _ := do(m, path, url.Values{"name": {"x"}, "action": {"ignore"}, "body": {"x"}}); res.StatusCode != http.StatusNotFound {
				t.Errorf("%s shared: %d", m, res.StatusCode)
			}
		}
		if _, err := shared.Get(cibID); err != nil {
			t.Fatalf("shared template changed: %v", err)
		}

		_, body = do("GET", "/sms/templates/new?copy="+cibID, nil)
		if !strings.Contains(body, "Credit card purchase (mine)") || !strings.Contains(body, "ending with{{account_id}}") || strings.Contains(body, `value="shared"`) {
			t.Error("override form isn't prefilled with the shared template as mine")
		}

		_, body = do("GET", "/sms/templates/new?"+url.Values{"sender": {"cib"}, "sms": {"Your OTP is 1234"}}.Encode(), nil)
		if !strings.Contains(body, `value="CIB"`) || strings.Count(body, "Your OTP is 1234") < 2 {
			t.Error("new form isn't prefilled with the sms, the sender and its bank")
		}

		res, _ = do("POST", "/sms/templates", url.Values{
			"scope": {"mine"}, "name": {"my otp"}, "action": {"ignore"}, "body": {"Your OTP is {{*}}"},
		})
		if res.Header.Get("HX-Redirect") != "/sms/templates" {
			t.Errorf("create mine: %d", res.StatusCode)
		}
	})
}
