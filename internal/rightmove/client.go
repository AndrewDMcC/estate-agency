// Package rightmove sends sales listings to Rightmove's Real Time Datafeed (RTDF).
//
// RTDF authenticates with a client TLS certificate issued by Rightmove, plus the
// agency's network and branch IDs. In dry-run mode payloads are only logged.
// Field names and code values follow the RTDF v1 property schema and should be
// checked against the specification Rightmove supplies with your credentials.
package rightmove

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/AndrewDMcC/estate-agency/internal/models"
)

type Mode string

const (
	ModeDryRun Mode = "dryrun"
	ModeTest   Mode = "test"
	ModeLive   Mode = "live"
)

const (
	TestAPIBase = "https://adfapi.adftest.rightmove.com/v1"
	LiveAPIBase = "https://adfapi.rightmove.co.uk/v1"

	pathSend   = "/property/sendpropertydetails"
	pathRemove = "/property/removeproperty"

	channelSales = 1
	mediaImage   = 1
)

type Config struct {
	Mode      Mode
	NetworkID int
	BranchID  int
	CertFile  string
	KeyFile   string
	CertPEM   string
	KeyPEM    string
	APIBase   string
}

type Client struct {
	cfg  Config
	base string
	http *http.Client
	log  *slog.Logger
}

func New(cfg Config, log *slog.Logger) (*Client, error) {
	c := &Client{cfg: cfg, log: log}
	switch cfg.Mode {
	case ModeDryRun, "":
		c.cfg.Mode = ModeDryRun
		return c, nil
	case ModeTest:
		c.base = TestAPIBase
	case ModeLive:
		c.base = LiveAPIBase
	default:
		return nil, fmt.Errorf("unknown rightmove mode %q (want dryrun, test or live)", cfg.Mode)
	}
	if cfg.APIBase != "" {
		c.base = strings.TrimRight(cfg.APIBase, "/")
	}
	if cfg.NetworkID == 0 || cfg.BranchID == 0 {
		return nil, errors.New("rightmove: network and branch IDs are required outside dry-run mode")
	}
	var cert tls.Certificate
	var err error
	switch {
	case cfg.CertPEM != "" && cfg.KeyPEM != "":
		cert, err = tls.X509KeyPair([]byte(cfg.CertPEM), []byte(cfg.KeyPEM))
	case cfg.CertFile != "" && cfg.KeyFile != "":
		cert, err = tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	default:
		return nil, errors.New("rightmove: client certificate and key are required outside dry-run mode")
	}
	if err != nil {
		return nil, fmt.Errorf("rightmove: load client certificate: %w", err)
	}
	c.http = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		},
	}
	return c, nil
}

func (c *Client) Mode() Mode   { return c.cfg.Mode }
func (c *Client) DryRun() bool { return c.cfg.Mode == ModeDryRun }

type Response struct {
	Success  bool              `json:"success"`
	Message  string            `json:"message"`
	Errors   []json.RawMessage `json:"errors"`
	Warnings []json.RawMessage `json:"warnings"`
}

func (r *Response) problems() string {
	var parts []string
	for _, e := range r.Errors {
		parts = append(parts, string(e))
	}
	if r.Message != "" {
		parts = append([]string{r.Message}, parts...)
	}
	return strings.Join(parts, "; ")
}

// SendProperty creates or updates a listing. baseURL makes relative photo paths absolute.
func (c *Client) SendProperty(ctx context.Context, p *models.Property, baseURL string) error {
	return c.post(ctx, pathSend, BuildSendRequest(c.cfg, p, baseURL))
}

func (c *Client) RemoveProperty(ctx context.Context, agentRef string) error {
	return c.post(ctx, pathRemove, removeRequest{
		Network:  network{NetworkID: c.cfg.NetworkID},
		Branch:   branch{BranchID: c.cfg.BranchID, Channel: channelSales},
		Property: removeProperty{AgentRef: agentRef},
	})
}

func (c *Client) post(ctx context.Context, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if c.DryRun() {
		c.log.Info("rightmove dry run", "endpoint", path, "payload", string(data))
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("rightmove request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("rightmove response: %w", err)
	}
	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("rightmove returned HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	if resp.StatusCode >= 300 || !out.Success {
		return fmt.Errorf("rightmove rejected request (HTTP %d): %s", resp.StatusCode, truncate(out.problems(), 500))
	}
	c.log.Info("rightmove request ok", "endpoint", path, "warnings", len(out.Warnings))
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

type network struct {
	NetworkID int `json:"network_id"`
}

type branch struct {
	BranchID int  `json:"branch_id"`
	Channel  int  `json:"channel"`
	Overseas bool `json:"overseas"`
}

type sendRequest struct {
	Network  network      `json:"network"`
	Branch   branch       `json:"branch"`
	Property sendProperty `json:"property"`
}

type sendProperty struct {
	AgentRef         string           `json:"agent_ref"`
	Published        bool             `json:"published"`
	PropertyType     int              `json:"property_type"`
	Status           int              `json:"status"`
	Address          address          `json:"address"`
	PriceInformation priceInformation `json:"price_information"`
	Details          details          `json:"details"`
	Media            []media          `json:"media,omitempty"`
}

type address struct {
	HouseNameNumber string  `json:"house_name_number"`
	Address2        string  `json:"address_2,omitempty"`
	Address3        string  `json:"address_3,omitempty"`
	Town            string  `json:"town"`
	Postcode1       string  `json:"postcode_1"`
	Postcode2       string  `json:"postcode_2"`
	DisplayAddress  string  `json:"display_address"`
	Latitude        float64 `json:"latitude,omitempty"`
	Longitude       float64 `json:"longitude,omitempty"`
}

type priceInformation struct {
	Price          int `json:"price"`
	PriceQualifier int `json:"price_qualifier"`
}

type details struct {
	Summary        string   `json:"summary"`
	Description    string   `json:"description"`
	Features       []string `json:"features,omitempty"`
	Bedrooms       int      `json:"bedrooms"`
	Bathrooms      int      `json:"bathrooms"`
	ReceptionRooms int      `json:"reception_rooms"`
}

type media struct {
	MediaType int    `json:"media_type"`
	MediaURL  string `json:"media_url"`
	SortOrder int    `json:"sort_order"`
}

type removeRequest struct {
	Network  network        `json:"network"`
	Branch   branch         `json:"branch"`
	Property removeProperty `json:"property"`
}

type removeProperty struct {
	AgentRef string `json:"agent_ref"`
}

// Rightmove property type codes (verify against the RTDF code list).
var propertyTypeCodes = map[string]int{
	"Terraced":       1,
	"End of Terrace": 2,
	"Semi-Detached":  3,
	"Detached":       4,
	"Flat":           8,
	"Maisonette":     11,
	"Bungalow":       12,
	"Land":           20,
	"Town House":     22,
	"Cottage":        23,
}

// Rightmove price qualifier codes (verify against the RTDF code list).
var priceQualifierCodes = map[string]int{
	"":                    0,
	"POA":                 1,
	"Guide Price":         2,
	"Fixed Price":         3,
	"Offers in Excess of": 4,
	"Offers in Region of": 5,
	"From":                7,
	"Offers Over":         10,
}

// Rightmove sales status codes (verify against the RTDF code list).
var statusCodes = map[models.Status]int{
	models.StatusAvailable:  1,
	models.StatusSSTC:       2,
	models.StatusUnderOffer: 4,
}

func BuildSendRequest(cfg Config, p *models.Property, baseURL string) sendRequest {
	pc1, pc2 := SplitPostcode(p.Postcode)
	req := sendRequest{
		Network: network{NetworkID: cfg.NetworkID},
		Branch:  branch{BranchID: cfg.BranchID, Channel: channelSales},
		Property: sendProperty{
			AgentRef:     p.AgentRef(),
			Published:    p.Live(),
			PropertyType: propertyTypeCodes[p.PropertyType],
			Status:       statusCodes[p.Status],
			Address: address{
				HouseNameNumber: p.HouseNumber,
				Address2:        p.Street,
				Address3:        p.Locality,
				Town:            p.Town,
				Postcode1:       pc1,
				Postcode2:       pc2,
				DisplayAddress:  p.Address(),
				Latitude:        p.Latitude,
				Longitude:       p.Longitude,
			},
			PriceInformation: priceInformation{Price: p.Price, PriceQualifier: priceQualifierCodes[p.PriceQualifier]},
			Details: details{
				Summary:        p.Summary,
				Description:    p.Description,
				Features:       p.Features,
				Bedrooms:       p.Bedrooms,
				Bathrooms:      p.Bathrooms,
				ReceptionRooms: p.Lounges,
			},
		},
	}
	base := strings.TrimRight(baseURL, "/")
	for i, photo := range p.Photos {
		if strings.HasPrefix(photo, "/") {
			photo = base + photo
		}
		req.Property.Media = append(req.Property.Media, media{MediaType: mediaImage, MediaURL: photo, SortOrder: i})
	}
	return req
}

var postcodeRe = regexp.MustCompile(`^([A-Z]{1,2}[0-9][A-Z0-9]?)\s*([0-9][A-Z]{2})$`)

// SplitPostcode splits a UK postcode into outward and inward codes.
func SplitPostcode(pc string) (string, string) {
	m := postcodeRe.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(pc)))
	if m == nil {
		return strings.TrimSpace(pc), ""
	}
	return m[1], m[2]
}

func ValidPostcode(pc string) bool {
	return postcodeRe.MatchString(strings.ToUpper(strings.TrimSpace(pc)))
}

// Validate lists problems that would stop Rightmove accepting the listing.
func Validate(p *models.Property) []string {
	var problems []string
	if p.HouseNumber == "" {
		problems = append(problems, "house name/number is required")
	}
	if p.Town == "" {
		problems = append(problems, "town is required")
	}
	if !ValidPostcode(p.Postcode) {
		problems = append(problems, "a full UK postcode is required")
	}
	if p.Price <= 0 && p.PriceQualifier != "POA" {
		problems = append(problems, "price is required")
	}
	if strings.TrimSpace(p.Summary) == "" {
		problems = append(problems, "summary is required")
	}
	if strings.TrimSpace(p.Description) == "" {
		problems = append(problems, "description is required")
	}
	if _, ok := propertyTypeCodes[p.PropertyType]; !ok {
		problems = append(problems, "property type is not mapped to a Rightmove code")
	}
	if _, ok := statusCodes[p.Status]; !ok {
		problems = append(problems, "status cannot be sent to Rightmove")
	}
	if len(p.Photos) == 0 {
		problems = append(problems, "at least one photo is required")
	}
	return problems
}
