package models

import "testing"

func TestRegistrationMatches(t *testing.T) {
	p := &Property{Price: 400000, Bedrooms: 3, Town: "Farnsfield", Locality: "Edingley", PropertyType: "Detached", Style: "Modern", MustHaves: []string{"Garden", "Garage"}}
	cases := []struct {
		name string
		reg  Registration
		want bool
	}{
		{"blank preferences", Registration{}, true},
		{"everything fits", Registration{MinPrice: 300000, MaxPrice: 450000, MinBedrooms: 3, Towns: []string{"farnsfield", "Southwell"},
			PropertyTypes: []string{"Detached"}, Styles: []string{"Modern"}, Ages: []string{"New build"}, MustHaves: []string{"Garden"}}, true},
		{"over budget", Registration{MaxPrice: 350000}, false},
		{"under minimum price", Registration{MinPrice: 450000}, false},
		{"too few bedrooms", Registration{MinBedrooms: 4}, false},
		{"other town", Registration{Towns: []string{"Southwell"}}, false},
		{"village matches", Registration{Towns: []string{"Edingley"}}, true},
		{"other type", Registration{PropertyTypes: []string{"Bungalow"}}, false},
		{"other style", Registration{Styles: []string{"Period"}}, false},
		{"missing must-have", Registration{MustHaves: []string{"Annexe"}}, false},
	}
	for _, c := range cases {
		if got := c.reg.Matches(p); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	if (&Registration{MustHaves: []string{"Garden"}}).Matches(&Property{Town: "Farnsfield"}) {
		t.Error("a property with no must-haves ticked should not match a buyer who needs one")
	}
	poa := &Property{PriceQualifier: "POA", Town: "Farnsfield"}
	if !(&Registration{MaxPrice: 100000}).Matches(poa) {
		t.Error("POA properties should not be excluded by price")
	}
}
