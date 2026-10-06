package models

import (
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusAvailable  Status = "available"
	StatusUnderOffer Status = "under_offer"
	StatusSSTC       Status = "sstc"
	StatusSold       Status = "sold"
)

var Statuses = []Status{StatusAvailable, StatusUnderOffer, StatusSSTC, StatusSold}

func (s Status) Label() string {
	switch s {
	case StatusUnderOffer:
		return "Under offer"
	case StatusSSTC:
		return "Sold STC"
	case StatusSold:
		return "Sold"
	default:
		return "For sale"
	}
}

func ValidStatus(s Status) bool {
	for _, v := range Statuses {
		if v == s {
			return true
		}
	}
	return false
}

var PropertyTypes = []string{
	"Detached", "Semi-Detached", "Terraced", "End of Terrace", "Town House",
	"Cottage", "Bungalow", "Flat", "Maisonette", "Land",
}

var PriceQualifiers = []string{
	"", "Guide Price", "Offers Over", "Offers in Excess of", "Offers in Region of",
	"Fixed Price", "From", "POA",
}

type Property struct {
	ID             string        `json:"id"`
	Ref            string        `json:"ref"`
	HouseNumber    string        `json:"house_number"`
	Street         string        `json:"street"`
	Locality       string        `json:"locality"`
	Town           string        `json:"town"`
	Postcode       string        `json:"postcode"`
	DisplayAddress string        `json:"display_address"`
	Price          int           `json:"price"`
	PriceQualifier string        `json:"price_qualifier"`
	PropertyType   string        `json:"property_type"`
	Bedrooms       int           `json:"bedrooms"`
	Bathrooms      int           `json:"bathrooms"`
	Lounges        int           `json:"lounges"`
	Summary        string        `json:"summary"`
	Description    string        `json:"description"`
	Features       []string      `json:"features"`
	Photos         []string      `json:"photos"`
	Latitude       float64       `json:"latitude"`
	Longitude      float64       `json:"longitude"`
	Status         Status        `json:"status"`
	Published      bool          `json:"published"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	Rightmove      RightmoveSync `json:"rightmove"`
}

type RightmoveSync struct {
	Listed     bool      `json:"listed"`
	LastSyncAt time.Time `json:"last_sync_at"`
	LastError  string    `json:"last_error"`
	DryRun     bool      `json:"dry_run"`
}

// AgentRef is the unique reference the agency uses for this property on portals.
func (p *Property) AgentRef() string {
	if p.Ref != "" {
		return p.Ref
	}
	return "P" + p.ID
}

func (p *Property) Address() string {
	if p.DisplayAddress != "" {
		return p.DisplayAddress
	}
	var parts []string
	for _, s := range []string{p.Street, p.Locality, p.Town} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

func (p *Property) FullAddress() string {
	var parts []string
	for _, s := range []string{strings.TrimSpace(p.HouseNumber + " " + p.Street), p.Locality, p.Town, p.Postcode} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

func (p *Property) MainPhoto() string {
	if len(p.Photos) > 0 {
		return p.Photos[0]
	}
	return ""
}

func (p *Property) PriceText() string {
	if p.PriceQualifier == "POA" {
		return "POA"
	}
	return FormatPrice(p.Price)
}

// Live reports whether the property should appear on the website and portals.
func (p *Property) Live() bool {
	return p.Published && p.Status != StatusSold
}

func (p *Property) HasLocation() bool {
	return p.Latitude != 0 || p.Longitude != 0
}

func FormatPrice(pounds int) string {
	s := fmt.Sprintf("%d", pounds)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return "£" + b.String()
}

type EnquiryKind string

const (
	EnquiryContact   EnquiryKind = "contact"
	EnquiryValuation EnquiryKind = "valuation"
)

func (k EnquiryKind) Label() string {
	if k == EnquiryValuation {
		return "Valuation"
	}
	return "Enquiry"
}

type Enquiry struct {
	ID         string      `json:"id"`
	Kind       EnquiryKind `json:"kind"`
	Name       string      `json:"name"`
	Email      string      `json:"email"`
	Phone      string      `json:"phone"`
	Address    string      `json:"address"`
	Message    string      `json:"message"`
	PropertyID string      `json:"property_id"`
	CreatedAt  time.Time   `json:"created_at"`
}

type Settings struct {
	AgencyName     string   `json:"agency_name"`
	Tagline        string   `json:"tagline"`
	LogoURL        string   `json:"logo_url"`
	PrimaryColor   string   `json:"primary_color"`
	AccentColor    string   `json:"accent_color"`
	Phone          string   `json:"phone"`
	Email          string   `json:"email"`
	OfficeAddress  string   `json:"office_address"`
	OpeningHours   string   `json:"opening_hours"`
	OfficeLat      float64  `json:"office_lat"`
	OfficeLng      float64  `json:"office_lng"`
	MapZoom        int      `json:"map_zoom"`
	HeroHeading    string   `json:"hero_heading"`
	HeroSubheading string   `json:"hero_subheading"`
	HeroImages     []string `json:"hero_images"`
	IntroHeading   string   `json:"intro_heading"`
	IntroText      string   `json:"intro_text"`
}

func (s Settings) PhoneHref() string {
	var b strings.Builder
	for _, c := range s.Phone {
		if c == '+' || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		}
	}
	return "tel:" + b.String()
}

func DefaultSettings() Settings {
	return Settings{
		AgencyName:     "Northgate Estates",
		Tagline:        "Independent estate agents",
		PrimaryColor:   "#12355b",
		AccentColor:    "#c8a165",
		Phone:          "01483 000 000",
		Email:          "hello@northgate-estates.example",
		OfficeAddress:  "12 High Street, Guildford, GU1 3AA",
		OpeningHours:   "Mon–Fri 9am–5:30pm · Sat 9am–4pm",
		OfficeLat:      51.2365,
		OfficeLng:      -0.5703,
		MapZoom:        14,
		HeroHeading:    "Find the home that fits your life",
		HeroSubheading: "Local, independent and focused on selling homes across Guildford and the surrounding villages.",
		IntroHeading:   "Local knowledge. Personal service.",
		IntroText: "We are a small, independent team of property professionals who live and work in the area we sell. " +
			"That means honest advice, accurate valuations and a sale that is managed personally from first viewing to completion.\n\n" +
			"Every home we sell is marketed on Rightmove and to our own register of qualified buyers, with professional photography, " +
			"floor plans and regular, straightforward updates.",
	}
}
