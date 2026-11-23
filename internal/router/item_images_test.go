package router_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/handler"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/receiptimg"
	"github.com/omareloui/tyche/internal/router"
	"github.com/omareloui/tyche/internal/sanitizer"
	"github.com/omareloui/tyche/internal/valid"
)

// TestItemImages sets, serves, replaces and deletes an item's picture, which
// follows a merge only when the target has none and goes with the item.
func TestItemImages(t *testing.T) {
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
	bank := newTestAccount(t, repo, ctx, "Bank", "EGP", models.AccKindBankAcc, 1000)
	on := tenantToday().AddDate(0, 0, -1)
	tx := &models.Transaction{
		ID: idgen.New(), Account: *bank, Kind: models.ExpenseTxKind, Amount: 15, ScheduledTo: on, FulfilledAt: on,
		Items: []models.TxItem{{Name: "Milk", Quantity: 1, Total: 10}, {Name: "Bread", Quantity: 1, Total: 5}},
	}
	if err := repo.CreateTransaction(ctx, tx, nil, nil); err != nil {
		t.Fatal(err)
	}
	lines, _ := repo.GetTxItems(ctx, tx.ID)
	milk, bread := lines[0].ItemID, lines[1].ItemID

	h := handler.New(repo, valid.New(), sanitizer.New(), nil, sharedTemplates(t), nil)
	srv := httptest.NewServer(router.New(h))
	t.Cleanup(srv.Close)
	send := func(req *http.Request) (*http.Response, []byte) {
		t.Helper()
		req.Header.Set("HX-Request", "true")
		req.AddCookie(&http.Cookie{Name: "access_token", Value: pair.Access.Encoded})
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, b
	}
	upload := func(id string, data []byte) (*http.Response, string) {
		t.Helper()
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("image", "photo.png")
		fw.Write(data)
		mw.Close()
		req, _ := http.NewRequest("PUT", srv.URL+"/item-pictures/"+id, &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		res, b := send(req)
		return res, string(b)
	}
	photo := func(w, h int, c color.RGBA) []byte {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for i := range w * h {
			img.SetRGBA(i%w, i/w, c)
		}
		var buf bytes.Buffer
		png.Encode(&buf, img)
		return buf.Bytes()
	}

	// Not an image: refused into the details' slot.
	res, body := upload(milk, []byte("nope"))
	if res.StatusCode != http.StatusUnprocessableEntity || res.Header.Get("HX-Retarget") != "#item-image-error" {
		t.Errorf("not an image: %d %q %q", res.StatusCode, res.Header.Get("HX-Retarget"), body)
	}

	// A big PNG is cropped and shrunk into a small square WebP, shown in the details.
	res, body = upload(milk, photo(2000, 1000, color.RGBA{200, 30, 30, 255}))
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("HX-Trigger"), "items-changed") || !strings.Contains(body, "Change picture") {
		t.Fatalf("set picture: %d %q", res.StatusCode, body)
	}
	m, err := repo.GetItem(ctx, milk)
	if err != nil || m.Item.Image == nil {
		t.Fatalf("milk has no picture: %v", err)
	}
	if m.Item.Image.Width != receiptimg.ThumbSide || m.Item.Image.Height != receiptimg.ThumbSide {
		t.Errorf("picture size = %d×%d", m.Item.Image.Width, m.Item.Image.Height)
	}
	if !strings.Contains(body, m.Item.ImageURL()) {
		t.Error("the details don't show the picture")
	}

	get, _ := http.NewRequest("GET", srv.URL+m.Item.ImageURL(), nil)
	res, img := send(get)
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/webp" || len(img) > receiptimg.ThumbMaxBytes {
		t.Errorf("serve picture: %d %q %d bytes", res.StatusCode, res.Header.Get("Content-Type"), len(img))
	}
	get, _ = http.NewRequest("GET", srv.URL+m.Item.ImageURL(), nil)
	get.Header.Set("If-None-Match", res.Header.Get("ETag"))
	if res, _ := send(get); res.StatusCode != http.StatusNotModified {
		t.Errorf("cached picture: %d, want 304", res.StatusCode)
	}

	// Replacing it changes its url.
	if res, _ := upload(milk, photo(300, 600, color.RGBA{30, 30, 200, 255})); res.StatusCode != 200 {
		t.Fatalf("replace picture: %d", res.StatusCode)
	}
	m2, _ := repo.GetItem(ctx, milk)
	if m2.Item.ImageURL() == m.Item.ImageURL() || m2.Item.Image.Width != receiptimg.ThumbSide {
		t.Errorf("replaced picture = %+v", m2.Item.Image)
	}

	// Merging keeps the target's picture, else takes the merged one's.
	if res, _ := upload(bread, photo(50, 50, color.RGBA{0, 200, 0, 255})); res.StatusCode != 200 {
		t.Fatal("bread picture")
	}
	b, _ := repo.GetItem(ctx, bread)
	if _, err := repo.MergeItems(ctx, bread, milk); err != nil {
		t.Fatal(err)
	}
	if m3, _ := repo.GetItem(ctx, milk); m3.Item.Image.Hash != m2.Item.Image.Hash {
		t.Error("merging replaced the target's picture")
	}
	if _, err := repo.GetItemImage(ctx, bread); err == nil {
		t.Error("the merged item's picture is still there")
	}

	// Deleting it.
	del, _ := http.NewRequest("DELETE", srv.URL+"/item-pictures/"+milk, nil)
	if res, body := send(del); res.StatusCode != 200 || !strings.Contains(string(body), "Add picture") {
		t.Errorf("delete picture: %d", res.StatusCode)
	}
	if m, _ := repo.GetItem(ctx, milk); m.Item.Image != nil {
		t.Error("the picture is still there")
	}

	// A merged item's picture moves to a target without one.
	tx2 := &models.Transaction{
		ID: idgen.New(), Account: *bank, Kind: models.ExpenseTxKind, Amount: 5, ScheduledTo: on, FulfilledAt: on,
		Items: []models.TxItem{{Name: "Eggs", Quantity: 1, Total: 5}},
	}
	if err := repo.CreateTransaction(ctx, tx2, nil, nil); err != nil {
		t.Fatal(err)
	}
	l2, _ := repo.GetTxItems(ctx, tx2.ID)
	eggs := l2[0].ItemID
	if err := repo.SetItemImage(ctx, eggs, []byte("webp"), 1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeItems(ctx, eggs, milk); err != nil {
		t.Fatal(err)
	}
	if m, _ := repo.GetItem(ctx, milk); m.Item.Image == nil || m.Item.Image.Hash == b.Item.Image.Hash {
		t.Errorf("merged picture = %+v", m.Item.Image)
	}

	// Resetting the items deletes the pictures.
	if err := repo.Reset(ctx, models.ResetOptions{models.ResetItems: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetItemImage(ctx, milk); err == nil {
		t.Error("a picture outlived the reset")
	}
}
