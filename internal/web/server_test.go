package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AndrewDMcC/estate-agency/internal/config"
	"github.com/AndrewDMcC/estate-agency/internal/rightmove"
	"github.com/AndrewDMcC/estate-agency/internal/store"
)

func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.SeedDemo(); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rm, _ := rightmove.New(rightmove.Config{Mode: rightmove.ModeDryRun}, log)
	cfg := config.Config{DataDir: dir, BaseURL: "http://example.test", AdminUser: "admin", AdminPassword: "secret"}
	srv, err := New(st, cfg, rm, log)
	if err != nil {
		t.Fatal(err)
	}
	return srv, st
}

func get(t *testing.T, h http.Handler, path string, auth bool) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if auth {
		req.SetBasicAuth("admin", "secret")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestPublicPages(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()
	code, body := get(t, h, "/", false)
	if code != 200 {
		t.Fatalf("home status %d: %s", code, body)
	}
	for _, want := range []string{"New properties for sale", "card__overlay", "office-map", "/captcha/", "Book a valuation", "--brand:#12355b"} {
		if !strings.Contains(body, want) {
			t.Errorf("home page missing %q", want)
		}
	}
	if n := strings.Count(body, `class="card"`); n != 5 {
		t.Errorf("expected 5 available cards on home page, got %d", n)
	}
	if code, _ := get(t, h, "/properties?beds=4", false); code != 200 {
		t.Errorf("properties status %d", code)
	}
	if code, body := get(t, h, "/properties/000001", false); code != 200 || !strings.Contains(body, "Arrange a viewing") {
		t.Errorf("property status %d", code)
	}
	if code, _ := get(t, h, "/nope", false); code != 404 {
		t.Errorf("expected 404, got %d", code)
	}
}

func TestEnquiryValidation(t *testing.T) {
	srv, st := newTestServer(t)
	form := url.Values{"kind": {"valuation"}, "name": {"Jo"}, "email": {"bad"}, "phone": {"123"}, "captcha_id": {"x"}, "captcha": {"1"}}
	req := httptest.NewRequest(http.MethodPost, "/enquiry", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"valid email", "valid phone", "address of the property", "code didn"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing error %q", want)
		}
	}
	if n, _ := st.CountEnquiries(); n != 0 {
		t.Errorf("invalid enquiry was saved")
	}
}

func TestEnquiryFormValidate(t *testing.T) {
	f := enquiryForm{Kind: "contact", Name: "Jo Bloggs", Email: "jo@example.com", Phone: "+44 7700 900123"}
	if errs := f.validate(); len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestAdmin(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()
	if code, _ := get(t, h, "/admin", false); code != 401 {
		t.Errorf("expected 401 without auth, got %d", code)
	}
	for _, path := range []string{"/admin", "/admin/properties/new", "/admin/properties/000001", "/admin/enquiries", "/admin/settings"} {
		if code, body := get(t, h, path, true); code != 200 {
			t.Errorf("%s status %d: %s", path, code, body)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/properties/000001/rightmove", strings.NewReader("csrf=wrong"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for bad csrf, got %d", rec.Code)
	}
}

func TestAdminSync(t *testing.T) {
	srv, st := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/admin/properties/000001/rightmove", strings.NewReader("csrf="+srv.csrf))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d: %s", rec.Code, rec.Body.String())
	}
	p, _ := st.GetProperty("000001")
	if !p.Rightmove.Pending {
		t.Fatalf("sync should queue the property: %+v", p.Rightmove)
	}
	srv.processRightmoveQueue(context.Background(), time.Now().UTC())
	p, _ = st.GetProperty("000001")
	if p.Rightmove.Pending || p.Rightmove.LastSyncAt.IsZero() || !p.Rightmove.DryRun || p.Rightmove.LastError != "" {
		t.Errorf("unexpected sync state: %+v", p.Rightmove)
	}
}
