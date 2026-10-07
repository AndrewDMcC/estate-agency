package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AndrewDMcC/estate-agency/internal/models"
)

func postForm(t *testing.T, h http.Handler, path string, form url.Values, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if auth {
		req.SetBasicAuth("admin", "secret")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRegisterPage(t *testing.T) {
	srv, _ := newTestServer(t)
	code, body := get(t, srv.Handler(), "/register", false)
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	for _, want := range []string{`href="/register"`, `name="title"`, `name="forename"`, `name="surname"`, "Farnsfield", "Bungalow", "Victorian / Edwardian", "Barn conversion", "/captcha/"} {
		if !strings.Contains(body, want) {
			t.Errorf("register page missing %q", want)
		}
	}
	if strings.Contains(body, "office-map") {
		t.Error("register page should not include the contact section")
	}
}

func TestRegisterValidation(t *testing.T) {
	srv, st := newTestServer(t)
	form := url.Values{"title": {"King"}, "forename": {""}, "surname": {"Smith"}, "email": {"x"}, "phone": {"1"},
		"min_price": {"500000"}, "max_price": {"200000"}, "towns": {"Farnsfield", "Atlantis"}, "captcha_id": {"x"}, "captcha": {"1"}}
	rec := postForm(t, srv.Handler(), "/register", form, false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"choose a title", "enter your forename", "valid email", "valid phone", "buying position", "maximum price", "contact you about matching", "code didn"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing error %q", want)
		}
	}
	if !strings.Contains(body, `value="Farnsfield" checked`) || strings.Contains(body, "Atlantis") {
		t.Error("submitted choices should be kept, unknown ones dropped")
	}
	if n, _ := st.CountRegistrations(); n != 0 {
		t.Error("invalid registration was saved")
	}
}

func TestRegisterFormValidate(t *testing.T) {
	f := registerForm{Registration: models.Registration{
		Title: "Mrs", Forename: "Jo", Surname: "Bloggs", Email: "jo@example.com", Phone: "01623 392676",
		Position: models.BuyerPositions[0], MinPrice: 200000, MaxPrice: 400000, Consent: true,
	}}
	if errs := f.validate(); len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestPicked(t *testing.T) {
	got := picked([]string{"C", "A", "Z"}, []string{"A", "B", "C"})
	if strings.Join(got, ",") != "A,C" {
		t.Errorf("got %v", got)
	}
}

func TestCleanOptions(t *testing.T) {
	got := cleanOptions(" Farnsfield \n\nSouthwell\nfarnsfield\r\nNewark ")
	if strings.Join(got, "|") != "Farnsfield|Southwell|Newark" {
		t.Errorf("got %q", got)
	}
}

func TestAdminRegisterOptions(t *testing.T) {
	srv, st := newTestServer(t)
	h := srv.Handler()
	for _, path := range []string{"/admin/registrations", "/admin/register-options"} {
		if code, body := get(t, h, path, true); code != 200 {
			t.Errorf("%s status %d: %s", path, code, body)
		}
	}
	form := url.Values{"csrf": {srv.csrf}, "property_types": {"Detached\nBungalow"}, "styles": {"Modern"}, "ages": {""}, "towns": {"Farnsfield\nEdingley"}}
	if rec := postForm(t, h, "/admin/register-options", form, true); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty list, got %d", rec.Code)
	}
	form.Set("ages", "New build")
	if rec := postForm(t, h, "/admin/register-options", form, true); rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d: %s", rec.Code, rec.Body.String())
	}
	opts, _ := st.GetRegisterOptions()
	if strings.Join(opts.Towns, ",") != "Farnsfield,Edingley" {
		t.Errorf("towns not saved: %v", opts.Towns)
	}
	_, body := get(t, h, "/register", false)
	if !strings.Contains(body, "Edingley") || strings.Contains(body, "Southwell") {
		t.Error("register page should use the saved options")
	}
	if err := st.SaveRegistration(&models.Registration{Title: "Mr", Forename: "Sam", Surname: "Lee", Towns: []string{"Farnsfield"}, MaxPrice: 300000}); err != nil {
		t.Fatal(err)
	}
	_, body = get(t, h, "/admin/registrations", true)
	if !strings.Contains(body, "Mr Sam Lee") || !strings.Contains(body, "Up to £300,000") {
		t.Error("registration not listed in admin")
	}
}
