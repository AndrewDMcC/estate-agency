package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AndrewDMcC/estate-agency/internal/models"
	"github.com/AndrewDMcC/estate-agency/internal/rightmove"
	"github.com/AndrewDMcC/estate-agency/internal/store"
)

const maxAdminBody = 64 << 20

var flashMessages = map[string]string{
	"created":  "Property created.",
	"saved":    "Property saved.",
	"deleted":  "Property deleted.",
	"synced":   "Rightmove sync finished. See the status panel for the result.",
	"settings": "Settings saved.",
	"options":  "Register options saved.",
}

type adminData struct {
	Settings      models.Settings
	Title         string
	Section       string
	Flash         string
	CSRF          string
	RightmoveMode string
	Data          any
}

type dashboardData struct {
	Properties        []*models.Property
	Enquiries         []*models.Enquiry
	EnquiryCount      int
	RegistrationCount int
	LiveCount         int
}

type propertyForm struct {
	Property          *models.Property
	IsNew             bool
	Errors            map[string]string
	PhotosText        string
	FeaturesText      string
	Statuses          []models.Status
	Types             []string
	Qualifiers        []string
	RightmoveProblems []string
}

type settingsForm struct {
	S        models.Settings
	HeroText string
	Errors   map[string]string
}

func (s *Server) adminRoutes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /admin", s.adminDashboard)
	m.HandleFunc("GET /admin/{$}", s.adminDashboard)
	m.HandleFunc("GET /admin/properties/new", s.adminNewProperty)
	m.HandleFunc("POST /admin/properties", s.adminCreateProperty)
	m.HandleFunc("GET /admin/properties/{id}", s.adminEditProperty)
	m.HandleFunc("POST /admin/properties/{id}", s.adminUpdateProperty)
	m.HandleFunc("POST /admin/properties/{id}/delete", s.adminDeleteProperty)
	m.HandleFunc("POST /admin/properties/{id}/rightmove", s.adminSyncProperty)
	m.HandleFunc("GET /admin/enquiries", s.adminEnquiries)
	m.HandleFunc("GET /admin/registrations", s.adminRegistrations)
	m.HandleFunc("GET /admin/register-options", s.adminRegisterOptions)
	m.HandleFunc("POST /admin/register-options", s.adminSaveRegisterOptions)
	m.HandleFunc("GET /admin/settings", s.adminSettings)
	m.HandleFunc("POST /admin/settings", s.adminSaveSettings)
	return s.requireAdmin(m)
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AdminPassword == "" {
			http.Error(w, "The admin area is disabled. Set ADMIN_PASSWORD to enable it.", http.StatusServiceUnavailable)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(s.cfg.AdminUser)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(s.cfg.AdminPassword)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Agency admin", charset="UTF-8"`)
			http.Error(w, "Unauthorised", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, maxAdminBody)
			if err := r.ParseMultipartForm(32 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
				http.Error(w, "Invalid form (uploads are limited to 64 MB in total)", http.StatusBadRequest)
				return
			}
			if subtle.ConstantTimeCompare([]byte(r.FormValue("csrf")), []byte(s.csrf)) != 1 {
				http.Error(w, "Invalid form token. Go back, refresh the page and try again.", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) renderAdmin(w http.ResponseWriter, r *http.Request, page, section, title string, data any, status int) {
	settings, err := s.store.GetSettings()
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.render(w, page, adminData{
		Settings: settings, Title: title, Section: section, CSRF: s.csrf,
		Flash:         flashMessages[r.URL.Query().Get("msg")],
		RightmoveMode: string(s.rm.Mode()),
		Data:          data,
	}, status)
}

func (s *Server) adminDashboard(w http.ResponseWriter, r *http.Request) {
	props, err := s.store.ListProperties(nil)
	if err != nil {
		s.serverError(w, err)
		return
	}
	enqs, err := s.store.ListEnquiries(5)
	if err != nil {
		s.serverError(w, err)
		return
	}
	count, _ := s.store.CountEnquiries()
	regCount, _ := s.store.CountRegistrations()
	live := 0
	for _, p := range props {
		if p.Live() {
			live++
		}
	}
	s.renderAdmin(w, r, "admin_dashboard", "dashboard", "Dashboard",
		dashboardData{Properties: props, Enquiries: enqs, EnquiryCount: count, RegistrationCount: regCount, LiveCount: live}, http.StatusOK)
}

func newPropertyForm(p *models.Property, isNew bool, errs map[string]string) propertyForm {
	f := propertyForm{
		Property: p, IsNew: isNew, Errors: errs,
		PhotosText: strings.Join(p.Photos, "\n"), FeaturesText: strings.Join(p.Features, "\n"),
		Statuses: models.Statuses, Types: models.PropertyTypes, Qualifiers: models.PriceQualifiers,
	}
	if !isNew && p.Live() {
		f.RightmoveProblems = rightmove.Validate(p)
	}
	return f
}

func (s *Server) adminNewProperty(w http.ResponseWriter, r *http.Request) {
	p := &models.Property{Status: models.StatusAvailable, Published: true, PropertyType: "Detached"}
	s.renderAdmin(w, r, "admin_property_form", "new", "Add property", newPropertyForm(p, true, nil), http.StatusOK)
}

func (s *Server) adminCreateProperty(w http.ResponseWriter, r *http.Request) {
	p := &models.Property{}
	if errs := s.bindProperty(r, p); len(errs) > 0 {
		s.renderAdmin(w, r, "admin_property_form", "new", "Add property", newPropertyForm(p, true, errs), http.StatusUnprocessableEntity)
		return
	}
	if err := s.store.SaveProperty(p); err != nil {
		s.serverError(w, err)
		return
	}
	s.syncRightmove(r.Context(), p)
	http.Redirect(w, r, "/admin/properties/"+p.ID+"?msg=created", http.StatusSeeOther)
}

func (s *Server) loadProperty(w http.ResponseWriter, r *http.Request) (*models.Property, bool) {
	p, err := s.store.GetProperty(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return nil, false
	}
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	return p, true
}

func (s *Server) adminEditProperty(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProperty(w, r)
	if !ok {
		return
	}
	s.renderAdmin(w, r, "admin_property_form", "dashboard", "Edit "+p.Address(), newPropertyForm(p, false, nil), http.StatusOK)
}

func (s *Server) adminUpdateProperty(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProperty(w, r)
	if !ok {
		return
	}
	if errs := s.bindProperty(r, p); len(errs) > 0 {
		s.renderAdmin(w, r, "admin_property_form", "dashboard", "Edit "+p.Address(), newPropertyForm(p, false, errs), http.StatusUnprocessableEntity)
		return
	}
	if err := s.store.SaveProperty(p); err != nil {
		s.serverError(w, err)
		return
	}
	s.syncRightmove(r.Context(), p)
	http.Redirect(w, r, "/admin/properties/"+p.ID+"?msg=saved", http.StatusSeeOther)
}

func (s *Server) adminDeleteProperty(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProperty(w, r)
	if !ok {
		return
	}
	if p.Rightmove.Listed {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := s.rm.RemoveProperty(ctx, p.AgentRef()); err != nil {
			s.log.Warn("rightmove remove failed", "id", p.ID, "err", err)
		}
	}
	if err := s.store.DeleteProperty(p.ID); err != nil {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin?msg=deleted", http.StatusSeeOther)
}

func (s *Server) adminSyncProperty(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProperty(w, r)
	if !ok {
		return
	}
	s.syncRightmove(r.Context(), p)
	http.Redirect(w, r, "/admin/properties/"+p.ID+"?msg=synced", http.StatusSeeOther)
}

// syncRightmove pushes live listings to Rightmove and removes ones that are no longer live.
func (s *Server) syncRightmove(ctx context.Context, p *models.Property) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var err error
	switch {
	case p.Live():
		if problems := rightmove.Validate(p); len(problems) > 0 {
			err = fmt.Errorf("not sent: %s", strings.Join(problems, "; "))
		} else if err = s.rm.SendProperty(ctx, p, s.cfg.BaseURL); err == nil {
			p.Rightmove.Listed = !s.rm.DryRun()
		}
	case p.Rightmove.Listed:
		if err = s.rm.RemoveProperty(ctx, p.AgentRef()); err == nil {
			p.Rightmove.Listed = false
		}
	default:
		return
	}
	p.Rightmove.LastSyncAt = time.Now().UTC()
	p.Rightmove.DryRun = s.rm.DryRun()
	p.Rightmove.LastError = ""
	if err != nil {
		p.Rightmove.LastError = err.Error()
		s.log.Warn("rightmove sync", "id", p.ID, "err", err)
	}
	if err := s.store.SaveProperty(p); err != nil {
		s.log.Error("save rightmove status", "id", p.ID, "err", err)
	}
}

func (s *Server) adminEnquiries(w http.ResponseWriter, r *http.Request) {
	enqs, err := s.store.ListEnquiries(0)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.renderAdmin(w, r, "admin_enquiries", "enquiries", "Enquiries", enqs, http.StatusOK)
}

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetSettings()
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.renderAdmin(w, r, "admin_settings", "settings", "Branding & settings",
		settingsForm{S: settings, HeroText: strings.Join(settings.HeroImages, "\n")}, http.StatusOK)
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (s *Server) adminSaveSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings()
	if err != nil {
		s.serverError(w, err)
		return
	}
	errs := map[string]string{}
	v := func(k string) string { return strings.TrimSpace(r.FormValue(k)) }
	st.AgencyName = v("agency_name")
	st.Tagline = v("tagline")
	st.Phone = v("phone")
	st.Email = v("email")
	st.OfficeAddress = v("office_address")
	st.OpeningHours = v("opening_hours")
	st.HeroHeading = v("hero_heading")
	st.HeroSubheading = v("hero_subheading")
	st.IntroHeading = v("intro_heading")
	st.IntroText = v("intro_text")
	if st.AgencyName == "" {
		errs["agency_name"] = "Agency name is required."
	}
	for key, dst := range map[string]*string{"primary_color": &st.PrimaryColor, "accent_color": &st.AccentColor} {
		if c := v(key); hexColor.MatchString(c) {
			*dst = c
		} else {
			errs[key] = "Use a hex colour such as #12355b."
		}
	}
	if lat, err := strconv.ParseFloat(v("office_lat"), 64); err == nil && lat >= -90 && lat <= 90 {
		st.OfficeLat = lat
	} else {
		errs["office_lat"] = "Enter a latitude between -90 and 90."
	}
	if lng, err := strconv.ParseFloat(v("office_lng"), 64); err == nil && lng >= -180 && lng <= 180 {
		st.OfficeLng = lng
	} else {
		errs["office_lng"] = "Enter a longitude between -180 and 180."
	}
	if z, err := strconv.Atoi(v("map_zoom")); err == nil && z >= 1 && z <= 19 {
		st.MapZoom = z
	} else {
		errs["map_zoom"] = "Zoom must be between 1 and 19."
	}
	hero, bad := splitURLs(v("hero_images"))
	if bad != "" {
		errs["hero_images"] = "Not a valid image URL: " + bad
	}
	uploaded, err := s.saveUploads(formFiles(r, "hero_files"))
	if err != nil {
		errs["hero_images"] = err.Error()
	}
	st.HeroImages = append(hero, uploaded...)
	if logo, err := s.saveUploads(formFiles(r, "logo_file")); err != nil {
		errs["logo"] = err.Error()
	} else if len(logo) > 0 {
		st.LogoURL = logo[0]
	}
	if r.FormValue("remove_logo") != "" {
		st.LogoURL = ""
	}
	if len(errs) > 0 {
		s.renderAdmin(w, r, "admin_settings", "settings", "Branding & settings",
			settingsForm{S: st, HeroText: strings.Join(st.HeroImages, "\n"), Errors: errs}, http.StatusUnprocessableEntity)
		return
	}
	if err := s.store.SaveSettings(st); err != nil {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/settings?msg=settings", http.StatusSeeOther)
}

func (s *Server) bindProperty(r *http.Request, p *models.Property) map[string]string {
	errs := map[string]string{}
	v := func(k string) string { return strings.TrimSpace(r.FormValue(k)) }
	p.Ref = v("ref")
	p.HouseNumber = v("house_number")
	p.Street = v("street")
	p.Locality = v("locality")
	p.Town = v("town")
	p.Postcode = strings.ToUpper(v("postcode"))
	p.DisplayAddress = v("display_address")
	p.PriceQualifier = oneOf(v("price_qualifier"), models.PriceQualifiers, "")
	p.PropertyType = oneOf(v("property_type"), models.PropertyTypes, models.PropertyTypes[0])
	p.Summary = v("summary")
	p.Description = v("description")
	p.Features = splitLines(v("features"))
	p.Published = r.FormValue("published") != ""
	p.Status = models.Status(v("status"))
	if !models.ValidStatus(p.Status) {
		p.Status = models.StatusAvailable
	}

	price, err := strconv.Atoi(strings.NewReplacer("£", "", ",", "", " ", "").Replace(v("price")))
	switch {
	case err != nil && v("price") != "":
		errs["price"] = "Enter the price in whole pounds, e.g. 425000."
	case price <= 0 && p.PriceQualifier != "POA":
		errs["price"] = "Price is required."
	default:
		p.Price = price
	}
	for key, dst := range map[string]*int{"bedrooms": &p.Bedrooms, "bathrooms": &p.Bathrooms, "lounges": &p.Lounges} {
		n, err := strconv.Atoi(v(key))
		if v(key) == "" {
			n, err = 0, nil
		}
		if err != nil || n < 0 || n > 50 {
			errs[key] = "Enter a number from 0 to 50."
			continue
		}
		*dst = n
	}
	p.Latitude, p.Longitude = 0, 0
	if v("latitude") != "" || v("longitude") != "" {
		lat, err1 := strconv.ParseFloat(v("latitude"), 64)
		lng, err2 := strconv.ParseFloat(v("longitude"), 64)
		if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
			errs["location"] = "Enter both latitude and longitude, or leave both blank."
		} else {
			p.Latitude, p.Longitude = lat, lng
		}
	}
	if p.Street == "" {
		errs["street"] = "Street is required."
	}
	if p.Town == "" {
		errs["town"] = "Town is required."
	}
	if !rightmove.ValidPostcode(p.Postcode) {
		errs["postcode"] = "Enter a full UK postcode, e.g. GU1 3AA."
	}

	photos, bad := splitURLs(v("photos"))
	if bad != "" {
		errs["photos"] = "Not a valid image URL: " + bad
	}
	uploaded, err := s.saveUploads(formFiles(r, "photo_files"))
	if err != nil {
		errs["photos"] = err.Error()
	}
	p.Photos = append(photos, uploaded...)
	return errs
}

func formFiles(r *http.Request, key string) []*multipart.FileHeader {
	if r.MultipartForm == nil {
		return nil
	}
	return r.MultipartForm.File[key]
}

var imageExts = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

func (s *Server) saveUploads(files []*multipart.FileHeader) ([]string, error) {
	var urls []string
	for _, fh := range files {
		if fh.Size == 0 {
			continue
		}
		url, err := s.saveUpload(fh)
		if err != nil {
			return urls, fmt.Errorf("%s: %w", fh.Filename, err)
		}
		urls = append(urls, url)
	}
	return urls, nil
}

func (s *Server) saveUpload(fh *multipart.FileHeader) (string, error) {
	f, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	ext, ok := imageExts[http.DetectContentType(head[:n])]
	if !ok {
		return "", errors.New("only JPEG, PNG, WebP or GIF images can be uploaded")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	name := hex.EncodeToString(buf) + ext
	out, err := os.OpenFile(filepath.Join(s.uploadDir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		return "", err
	}
	return "/uploads/" + name, out.Close()
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// splitURLs parses one image URL per line, returning the first invalid entry if any.
func splitURLs(s string) ([]string, string) {
	var out []string
	for _, u := range splitLines(s) {
		if !strings.HasPrefix(u, "/uploads/") && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
			return out, u
		}
		out = append(out, u)
	}
	return out, ""
}

func oneOf(v string, allowed []string, fallback string) string {
	for _, a := range allowed {
		if a == v {
			return v
		}
	}
	return fallback
}
