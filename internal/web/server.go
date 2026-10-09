package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dchest/captcha"

	"github.com/AndrewDMcC/estate-agency/internal/config"
	"github.com/AndrewDMcC/estate-agency/internal/models"
	"github.com/AndrewDMcC/estate-agency/internal/store"
)

type Server struct {
	store     *store.Store
	cfg       config.Config
	rm        rightmoveClient
	wake      chan struct{}
	log       *slog.Logger
	tmpl      map[string]*template.Template
	uploadDir string
	csrf      string
}

func New(st *store.Store, cfg config.Config, rm rightmoveClient, log *slog.Logger) (*Server, error) {
	tmpl, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	secret, err := st.Secret("csrf")
	if err != nil {
		return nil, err
	}
	uploadDir := filepath.Join(cfg.DataDir, "uploads")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return nil, err
	}
	return &Server{store: st, cfg: cfg, rm: rm, log: log, tmpl: tmpl, uploadDir: uploadDir, csrf: secret, wake: make(chan struct{}, 1)}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	staticFS, _ := fs.Sub(assets, "static")
	mux.Handle("GET /static/", cacheFor(time.Hour, http.StripPrefix("/static/", http.FileServerFS(staticFS))))
	mux.Handle("GET /uploads/", cacheFor(24*time.Hour, http.StripPrefix("/uploads/", noDirListing(http.FileServer(http.Dir(s.uploadDir))))))
	mux.Handle("GET /captcha/", captcha.Server(captcha.StdWidth, captcha.StdHeight))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /properties", s.handleProperties)
	mux.HandleFunc("GET /properties/{id}", s.handleProperty)
	mux.HandleFunc("POST /enquiry", s.handleEnquiry)
	mux.HandleFunc("GET /register", s.handleRegister)
	mux.HandleFunc("POST /register", s.handleRegisterSubmit)

	admin := s.adminRoutes()
	mux.Handle("/admin", admin)
	mux.Handle("/admin/", admin)

	mux.HandleFunc("/", s.handleNotFound)
	return s.logRequests(securityHeaders(mux))
}

type pageData struct {
	Settings    models.Settings
	Title       string
	Hero        bool
	HideContact bool
	Form        enquiryForm
	CaptchaID   string
	Sent        bool
	MapMarkers  string
	Year        int
	Data        any
}

type enquiryForm struct {
	Kind       string
	Name       string
	Email      string
	Phone      string
	Address    string
	Message    string
	PropertyID string
	Errors     map[string]string
}

func (f *enquiryForm) validate() map[string]string {
	errs := map[string]string{}
	if f.Name == "" {
		errs["name"] = "Please tell us your name."
	} else if len(f.Name) > 120 {
		errs["name"] = "Name is too long."
	}
	if !validEmail(f.Email) {
		errs["email"] = "Please enter a valid email address."
	}
	if !validPhone(f.Phone) {
		errs["phone"] = "Please enter a valid phone number."
	}
	if f.Kind == string(models.EnquiryValuation) && f.Address == "" {
		errs["address"] = "Please enter the address of the property to value."
	} else if len(f.Address) > 300 {
		errs["address"] = "Address is too long."
	}
	if len(f.Message) > 3000 {
		errs["message"] = "Message is too long."
	}
	return errs
}

func validEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email && len(email) <= 254
}

func validPhone(phone string) bool {
	digits := 0
	for _, c := range phone {
		switch {
		case c >= '0' && c <= '9':
			digits++
		case strings.ContainsRune(" +()-", c):
		default:
			return false
		}
	}
	return digits >= 10 && digits <= 13
}

type homeData struct {
	NewProperties []*models.Property
}

type listData struct {
	Properties   []*models.Property
	MinBeds      int
	MaxPrice     int
	Sort         string
	BedOptions   []int
	PriceOptions []int
}

type marker struct {
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Address string  `json:"address"`
	Price   string  `json:"price"`
	URL     string  `json:"url"`
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	s.renderHome(w, r, enquiryForm{}, http.StatusOK)
}

func (s *Server) renderHome(w http.ResponseWriter, r *http.Request, form enquiryForm, status int) {
	props, err := s.store.ListProperties(func(p *models.Property) bool {
		return p.Live() && p.Status == models.StatusAvailable
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	if len(props) > 6 {
		props = props[:6]
	}
	s.renderPublic(w, r, "home", pageData{Hero: true, Form: form, Data: homeData{NewProperties: props}}, status)
}

func (s *Server) handleProperties(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	minBeds, _ := strconv.Atoi(q.Get("beds"))
	maxPrice, _ := strconv.Atoi(q.Get("max_price"))
	sortBy := q.Get("sort")
	props, err := s.store.ListProperties(func(p *models.Property) bool {
		return p.Live() && p.Bedrooms >= minBeds && (maxPrice <= 0 || p.Price <= maxPrice)
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	switch sortBy {
	case "price_asc":
		sort.SliceStable(props, func(i, j int) bool { return props[i].Price < props[j].Price })
	case "price_desc":
		sort.SliceStable(props, func(i, j int) bool { return props[i].Price > props[j].Price })
	default:
		sortBy = "newest"
	}
	s.renderPublic(w, r, "properties", pageData{Title: "Properties for sale", Data: listData{
		Properties: props, MinBeds: minBeds, MaxPrice: maxPrice, Sort: sortBy,
		BedOptions:   []int{1, 2, 3, 4, 5},
		PriceOptions: []int{250000, 400000, 500000, 750000, 1000000, 1500000},
	}}, http.StatusOK)
}

func (s *Server) handleProperty(w http.ResponseWriter, r *http.Request) {
	p, err := s.store.GetProperty(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) || (err == nil && !p.Live()) {
		s.handleNotFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.renderProperty(w, r, p, enquiryForm{
		PropertyID: p.ID,
		Message:    "I'd like to arrange a viewing of " + p.Address() + ".",
	}, http.StatusOK)
}

func (s *Server) renderProperty(w http.ResponseWriter, r *http.Request, p *models.Property, form enquiryForm, status int) {
	s.renderPublic(w, r, "property", pageData{Title: p.Address(), Form: form, Data: p}, status)
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.renderPublic(w, r, "notfound", pageData{Title: "Page not found"}, http.StatusNotFound)
}

func (s *Server) handleEnquiry(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	v := func(k string) string { return strings.TrimSpace(r.PostFormValue(k)) }
	f := enquiryForm{
		Kind: v("kind"), Name: v("name"), Email: v("email"), Phone: v("phone"),
		Address: v("address"), Message: v("message"), PropertyID: v("property_id"),
	}
	if f.Kind != string(models.EnquiryValuation) {
		f.Kind = string(models.EnquiryContact)
	}
	back := "/"
	var prop *models.Property
	if f.PropertyID != "" {
		if p, err := s.store.GetProperty(f.PropertyID); err == nil && p.Live() {
			prop, back = p, "/properties/"+p.ID
		} else {
			f.PropertyID = ""
		}
	}
	if v("website") != "" { // honeypot field: bots fill it, people never see it
		http.Redirect(w, r, back+"?sent=1#contact", http.StatusSeeOther)
		return
	}
	f.Errors = f.validate()
	if !captcha.VerifyString(v("captcha_id"), v("captcha")) {
		f.Errors["captcha"] = "The code didn't match. Please enter the new code shown."
	}
	if len(f.Errors) > 0 {
		if prop != nil {
			s.renderProperty(w, r, prop, f, http.StatusUnprocessableEntity)
		} else {
			s.renderHome(w, r, f, http.StatusUnprocessableEntity)
		}
		return
	}
	e := &models.Enquiry{
		Kind: models.EnquiryKind(f.Kind), Name: f.Name, Email: f.Email, Phone: f.Phone,
		Address: f.Address, Message: f.Message, PropertyID: f.PropertyID,
	}
	if err := s.store.SaveEnquiry(e); err != nil {
		s.serverError(w, err)
		return
	}
	s.log.Info("new enquiry", "id", e.ID, "kind", e.Kind)
	http.Redirect(w, r, back+"?sent=1#contact", http.StatusSeeOther)
}

func (s *Server) renderPublic(w http.ResponseWriter, r *http.Request, page string, pd pageData, status int) {
	settings, err := s.store.GetSettings()
	if err != nil {
		s.serverError(w, err)
		return
	}
	pd.Settings = settings
	if pd.Form.Kind == "" {
		pd.Form.Kind = string(models.EnquiryContact)
		if r.URL.Query().Get("kind") == string(models.EnquiryValuation) {
			pd.Form.Kind = string(models.EnquiryValuation)
		}
	}
	pd.CaptchaID = captcha.New()
	pd.Sent = r.URL.Query().Get("sent") == "1" && pd.Form.Errors == nil
	pd.MapMarkers = s.mapMarkers()
	pd.Year = time.Now().Year()
	s.render(w, page, pd, status)
}

func (s *Server) mapMarkers() string {
	props, err := s.store.ListProperties(func(p *models.Property) bool { return p.Live() && p.HasLocation() })
	if err != nil {
		s.log.Error("map markers", "err", err)
		return "[]"
	}
	markers := make([]marker, 0, len(props))
	for _, p := range props {
		markers = append(markers, marker{Lat: p.Latitude, Lng: p.Longitude, Address: p.Address(), Price: p.PriceText(), URL: "/properties/" + p.ID})
	}
	data, _ := json.Marshal(markers)
	return string(data)
}

func (s *Server) render(w http.ResponseWriter, name string, data any, status int) {
	t, ok := s.tmpl[name]
	if !ok {
		s.serverError(w, errors.New("unknown template "+name))
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		s.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	s.log.Error("server error", "err", err)
	http.Error(w, "Something went wrong. Please try again.", http.StatusInternalServerError)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; script-src 'self'; form-action 'self'; base-uri 'self'; object-src 'none'")
		next.ServeHTTP(w, r)
	})
}

func cacheFor(d time.Duration, next http.Handler) http.Handler {
	v := "public, max-age=" + strconv.Itoa(int(d.Seconds()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", v)
		next.ServeHTTP(w, r)
	})
}

func noDirListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/static/") {
			return
		}
		s.log.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "dur", time.Since(start).Round(time.Millisecond))
	})
}
