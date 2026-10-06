package rightmove

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/AndrewDMcC/estate-agency/internal/models"
)

func TestSplitPostcode(t *testing.T) {
	cases := map[string][2]string{
		"GU1 3AA":  {"GU1", "3AA"},
		"sw1a1aa":  {"SW1A", "1AA"},
		" M1 1AE ": {"M1", "1AE"},
		"bad":      {"bad", ""},
	}
	for in, want := range cases {
		a, b := SplitPostcode(in)
		if a != want[0] || b != want[1] {
			t.Errorf("SplitPostcode(%q) = %q, %q; want %q, %q", in, a, b, want[0], want[1])
		}
	}
}

func TestBuildSendRequest(t *testing.T) {
	p := &models.Property{
		ID: "000001", HouseNumber: "4", Street: "Orchard Close", Town: "Guildford", Postcode: "GU1 2RT",
		Price: 875000, PriceQualifier: "Guide Price", PropertyType: "Detached",
		Bedrooms: 4, Bathrooms: 3, Lounges: 2, Status: models.StatusAvailable, Published: true,
		Photos: []string{"/uploads/a.jpg", "https://cdn.example/b.jpg"},
	}
	req := BuildSendRequest(Config{NetworkID: 10, BranchID: 20}, p, "https://agency.example/")
	data, _ := json.Marshal(req)
	got := string(data)
	for _, want := range []string{
		`"network_id":10`, `"branch_id":20`, `"channel":1`, `"agent_ref":"P000001"`,
		`"postcode_1":"GU1"`, `"postcode_2":"2RT"`, `"price":875000`, `"price_qualifier":2`,
		`"property_type":4`, `"reception_rooms":2`, `"media_url":"https://agency.example/uploads/a.jpg"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("payload missing %s\n%s", want, got)
		}
	}
}

func TestDryRunAndValidate(t *testing.T) {
	c, err := New(Config{Mode: ModeDryRun}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if !c.DryRun() {
		t.Fatal("expected dry run")
	}
	p := &models.Property{Status: models.StatusAvailable}
	if len(Validate(p)) == 0 {
		t.Fatal("expected validation problems for empty property")
	}
	if err := c.SendProperty(context.Background(), p, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{Mode: ModeLive}, slog.Default()); err == nil {
		t.Fatal("expected error for live mode without credentials")
	}
}
