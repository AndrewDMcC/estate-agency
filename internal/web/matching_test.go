package web

import (
	"net/url"
	"strings"
	"testing"

	"github.com/AndrewDMcC/estate-agency/internal/models"
)

func TestMatchingBuyersShown(t *testing.T) {
	srv, st := newTestServer(t)
	h := srv.Handler()
	match := &models.Registration{Title: "Mr", Forename: "Matt", Surname: "Ching", Email: "m@example.com", Phone: "07700900000",
		Position: "Cash buyer", MaxPrice: 900000, MinBedrooms: 4, Towns: []string{"Guildford"}, PropertyTypes: []string{"Detached"}, Consent: true}
	other := &models.Registration{Title: "Ms", Forename: "Nora", Surname: "Match", Email: "n@example.com", Phone: "07700900001",
		Position: "Cash buyer", PropertyTypes: []string{"Bungalow"}, Consent: true}
	for _, r := range []*models.Registration{match, other} {
		if err := st.SaveRegistration(r); err != nil {
			t.Fatal(err)
		}
	}
	_, body := get(t, h, "/admin/properties/000001", true)
	if !strings.Contains(body, "Matching buyers (1)") || !strings.Contains(body, "Mr Matt Ching") || strings.Contains(body, "Nora") {
		t.Errorf("property page should list only the matching buyer:\n%s", body)
	}
	_, body = get(t, h, "/admin/registrations", true)
	if !strings.Contains(body, `id="reg-`+match.ID+`"`) || !strings.Contains(body, "Matching properties (1)") || !strings.Contains(body, "Orchard Close") {
		t.Errorf("registrations page should list matching properties:\n%s", body)
	}
}

func TestPropertyFormUsesRegisterOptions(t *testing.T) {
	srv, st := newTestServer(t)
	h := srv.Handler()
	_, body := get(t, h, "/admin/properties/000001", true)
	for _, want := range []string{"Flat / apartment", "Barn conversion", "Victorian / Edwardian", ">Farnsfield<", `value="Garden"`, ">Guildford<"} {
		if !strings.Contains(body, want) {
			t.Errorf("property form missing %q", want)
		}
	}
	form := url.Values{"csrf": {srv.csrf}, "street": {"Main Street"}, "town": {"Farnsfield"}, "postcode": {"NG22 8EA"}, "price": {"350000"},
		"property_type": {"Flat / apartment"}, "style": {"Modern"}, "age": {"New build"}, "must_haves": {"Garden", "Swimming pool"}, "status": {"available"}, "published": {"1"}}
	if rec := postForm(t, h, "/admin/properties/000001", form, true); rec.Code != 303 {
		t.Fatalf("expected redirect, got %d: %s", rec.Code, rec.Body.String())
	}
	p, _ := st.GetProperty("000001")
	if p.Town != "Farnsfield" || p.PropertyType != "Flat / apartment" || p.Style != "Modern" || p.Age != "New build" || len(p.MustHaves) != 1 {
		t.Errorf("unexpected property: %+v", p)
	}
	form.Set("town", "Atlantis")
	form.Set("property_type", "Spaceship")
	rec := postForm(t, h, "/admin/properties/000001", form, true)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "Choose a town") || !strings.Contains(rec.Body.String(), "Choose a property type") {
		t.Errorf("unknown town/type should be rejected, got %d", rec.Code)
	}
}
