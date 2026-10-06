package store

import (
	"time"

	"github.com/AndrewDMcC/estate-agency/internal/models"
)

func unsplash(id string, w int) string {
	return "https://images.unsplash.com/" + id + "?auto=format&fit=crop&w=" + itoa(w) + "&q=80"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// SeedDemo populates an empty database with placeholder branding and listings.
// It runs once: after the first seed the settings record exists and seeding is skipped.
func (s *Store) SeedDemo() (bool, error) {
	seeded, err := s.hasSettings()
	if err != nil || seeded {
		return false, err
	}
	settings := models.DefaultSettings()
	settings.HeroImages = []string{
		unsplash("photo-1600596542815-ffad4c1539a9", 1920),
		unsplash("photo-1564013799919-ab600027ffc6", 1920),
		unsplash("photo-1600585154340-be6161a56a0c", 1920),
		unsplash("photo-1512917774080-9991f1c4c750", 1920),
	}
	if err := s.SaveSettings(settings); err != nil {
		return false, err
	}

	interiors := [][]string{
		{"photo-1600607687939-ce8a6c25118c", "photo-1600566753190-17f0baa2a6c3"},
		{"photo-1502672260266-1c1ef2d93688", "photo-1484154218962-a197022b5858"},
		{"photo-1522708323590-d24dbb6b0267", "photo-1600607687939-ce8a6c25118c"},
	}
	demo := []models.Property{
		{Ref: "NGE-1001", HouseNumber: "4", Street: "Orchard Close", Locality: "Merrow", Town: "Guildford", Postcode: "GU1 2RT",
			Price: 875000, PriceQualifier: "Guide Price", PropertyType: "Detached", Bedrooms: 4, Bathrooms: 3, Lounges: 2,
			Latitude: 51.2440, Longitude: -0.5290, Photos: []string{"photo-1568605114967-8130f3a36994"},
			Summary:  "A beautifully presented four bedroom detached family home in a quiet cul-de-sac.",
			Features: []string{"Four double bedrooms", "Two en-suites", "Landscaped rear garden", "Double garage", "Walk to village shops"}},
		{Ref: "NGE-1002", HouseNumber: "27", Street: "Victoria Road", Town: "Guildford", Postcode: "GU1 4DJ",
			Price: 525000, PropertyType: "Terraced", Bedrooms: 3, Bathrooms: 1, Lounges: 1,
			Latitude: 51.2395, Longitude: -0.5600, Photos: []string{"photo-1570129477492-45c003edd2be"},
			Summary:  "A charming Victorian terrace moments from the town centre and mainline station.",
			Features: []string{"Period features", "Open-plan kitchen", "South-facing garden", "No onward chain"}},
		{Ref: "NGE-1003", HouseNumber: "Flat 6", Street: "Riverside Court", Town: "Guildford", Postcode: "GU1 4RQ",
			Price: 315000, PriceQualifier: "Offers Over", PropertyType: "Flat", Bedrooms: 2, Bathrooms: 2, Lounges: 1,
			Latitude: 51.2330, Longitude: -0.5770, Photos: []string{"photo-1545324418-cc1a3fa10c00"},
			Summary:  "A bright two bedroom apartment with balcony overlooking the River Wey.",
			Features: []string{"Private balcony", "Allocated parking", "Lift access", "Long lease"}},
		{Ref: "NGE-1004", HouseNumber: "The Old Forge", Street: "The Street", Locality: "Shalford", Town: "Guildford", Postcode: "GU4 8BU",
			Price: 1250000, PriceQualifier: "Offers in Excess of", PropertyType: "Cottage", Bedrooms: 5, Bathrooms: 3, Lounges: 3,
			Latitude: 51.2150, Longitude: -0.5660, Photos: []string{"photo-1580587771525-78b9dba3b914"},
			Summary:  "An exceptional period home set in half an acre in the heart of the village.",
			Features: []string{"Half an acre", "Inglenook fireplace", "Home office", "Detached studio"}},
		{Ref: "NGE-1005", HouseNumber: "12", Street: "Beech Avenue", Locality: "Burpham", Town: "Guildford", Postcode: "GU4 7LP",
			Price: 640000, PropertyType: "Semi-Detached", Bedrooms: 3, Bathrooms: 2, Lounges: 2,
			Latitude: 51.2580, Longitude: -0.5420, Photos: []string{"photo-1605276374104-dee2a0ed3cd6"},
			Summary:  "A spacious extended semi-detached home close to well-regarded schools.",
			Features: []string{"Extended kitchen/family room", "Driveway parking", "Downstairs cloakroom", "Loft potential"}},
		{Ref: "NGE-1006", HouseNumber: "9", Street: "Pewley Hill", Town: "Guildford", Postcode: "GU1 3SN",
			Price: 995000, PropertyType: "Detached", Bedrooms: 4, Bathrooms: 2, Lounges: 2, Status: models.StatusUnderOffer,
			Latitude: 51.2340, Longitude: -0.5650, Photos: []string{"photo-1613490493576-7fde63acd811"},
			Summary:  "A striking contemporary home with far-reaching views over the town.",
			Features: []string{"Panoramic views", "Underfloor heating", "Bi-fold doors", "EV charger"}},
	}
	base := time.Now().UTC().Add(-time.Duration(len(demo)) * 24 * time.Hour)
	for i := range demo {
		p := demo[i]
		photos := []string{unsplash(p.Photos[0], 1200)}
		for _, id := range interiors[i%len(interiors)] {
			photos = append(photos, unsplash(id, 1200))
		}
		p.Photos = photos
		if p.Status == "" {
			p.Status = models.StatusAvailable
		}
		p.Published = true
		p.Description = p.Summary + "\n\nThe accommodation is arranged over two floors and has been well maintained by the current owners. " +
			"Viewing is highly recommended to appreciate the space and setting on offer.\n\nPlaceholder listing for demonstration purposes."
		if err := s.SaveProperty(&p); err != nil {
			return false, err
		}
		p.CreatedAt = base.Add(time.Duration(i) * 24 * time.Hour)
		if err := s.SaveProperty(&p); err != nil {
			return false, err
		}
	}
	return true, nil
}
