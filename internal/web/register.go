package web

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dchest/captcha"

	"github.com/AndrewDMcC/estate-agency/internal/models"
)

var registerPrices = []int{100000, 150000, 200000, 250000, 300000, 350000, 400000, 500000, 600000, 750000, 1000000, 1500000, 2000000}

type registerForm struct {
	models.Registration
	Errors map[string]string
}

type choiceGroup struct {
	Name     string
	Label    string
	Options  []string
	Selected []string
}

type registerPage struct {
	Form         registerForm
	Titles       []string
	Positions    []string
	PriceOptions []int
	BedOptions   []int
	Groups       []choiceGroup
}

func (f *registerForm) validate() map[string]string {
	errs := map[string]string{}
	if !slices.Contains(models.Titles, f.Title) {
		errs["title"] = "Please choose a title."
	}
	for key, val := range map[string]string{"forename": f.Forename, "surname": f.Surname} {
		if val == "" {
			errs[key] = "Please enter your " + key + "."
		} else if utf8.RuneCountInString(val) > 60 {
			errs[key] = "This is too long."
		}
	}
	if !validEmail(f.Email) {
		errs["email"] = "Please enter a valid email address."
	}
	if !validPhone(f.Phone) {
		errs["phone"] = "Please enter a valid phone number."
	}
	if len(f.Address) > 300 {
		errs["address"] = "Address is too long."
	}
	if len(f.Postcode) > 10 {
		errs["postcode"] = "Please enter a valid postcode."
	}
	if !slices.Contains(models.BuyerPositions, f.Position) {
		errs["position"] = "Please tell us your buying position."
	}
	if f.MinPrice < 0 || f.MaxPrice < 0 || (f.MaxPrice > 0 && f.MinPrice > f.MaxPrice) {
		errs["price"] = "The maximum price must be higher than the minimum price."
	}
	if f.MinBedrooms < 0 || f.MinBedrooms > 10 {
		errs["bedrooms"] = "Please choose the number of bedrooms."
	}
	if len(f.Notes) > 2000 {
		errs["notes"] = "Notes are too long."
	}
	if !f.Consent {
		errs["consent"] = "Please confirm we may contact you about matching properties."
	}
	return errs
}

// picked keeps the submitted values that are valid choices, in the order of allowed.
func picked(values, allowed []string) []string {
	var out []string
	for _, a := range allowed {
		if slices.Contains(values, a) {
			out = append(out, a)
		}
	}
	return out
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	s.renderRegister(w, r, registerForm{}, http.StatusOK)
}

func (s *Server) renderRegister(w http.ResponseWriter, r *http.Request, f registerForm, status int) {
	opts, err := s.store.GetRegisterOptions()
	if err != nil {
		s.serverError(w, err)
		return
	}
	page := registerPage{
		Form:         f,
		Titles:       models.Titles,
		Positions:    models.BuyerPositions,
		PriceOptions: registerPrices,
		BedOptions:   []int{1, 2, 3, 4, 5},
		Groups: []choiceGroup{
			{Name: "property_types", Label: "Property type", Options: opts.PropertyTypes, Selected: f.PropertyTypes},
			{Name: "styles", Label: "Style", Options: opts.Styles, Selected: f.Styles},
			{Name: "ages", Label: "Age", Options: opts.Ages, Selected: f.Ages},
			{Name: "towns", Label: "Towns and villages", Options: opts.Towns, Selected: f.Towns},
			{Name: "must_haves", Label: "Must-haves", Options: models.MustHaves, Selected: f.MustHaves},
		},
	}
	s.renderPublic(w, r, "register", pageData{Title: "Register", HideContact: true, Form: enquiryForm{Errors: f.Errors}, Data: page}, status)
}

func (s *Server) handleRegisterSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	opts, err := s.store.GetRegisterOptions()
	if err != nil {
		s.serverError(w, err)
		return
	}
	v := func(k string) string { return strings.TrimSpace(r.PostFormValue(k)) }
	n := func(k string) int { i, _ := strconv.Atoi(v(k)); return i }
	f := registerForm{Registration: models.Registration{
		Title: v("title"), Forename: v("forename"), Surname: v("surname"),
		Email: v("email"), Phone: v("phone"), Address: v("address"), Postcode: strings.ToUpper(v("postcode")),
		Position: v("position"), MinPrice: n("min_price"), MaxPrice: n("max_price"), MinBedrooms: n("min_bedrooms"),
		PropertyTypes: picked(r.PostForm["property_types"], opts.PropertyTypes),
		Styles:        picked(r.PostForm["styles"], opts.Styles),
		Ages:          picked(r.PostForm["ages"], opts.Ages),
		Towns:         picked(r.PostForm["towns"], opts.Towns),
		MustHaves:     picked(r.PostForm["must_haves"], models.MustHaves),
		Notes:         v("notes"),
		Consent:       v("consent") != "",
	}}
	if v("website") != "" { // honeypot
		http.Redirect(w, r, "/register?sent=1", http.StatusSeeOther)
		return
	}
	f.Errors = f.validate()
	if !captcha.VerifyString(v("captcha_id"), v("captcha")) {
		f.Errors["captcha"] = "The code didn't match. Please enter the new code shown."
	}
	if len(f.Errors) > 0 {
		s.renderRegister(w, r, f, http.StatusUnprocessableEntity)
		return
	}
	reg := f.Registration
	if err := s.store.SaveRegistration(&reg); err != nil {
		s.serverError(w, err)
		return
	}
	s.log.Info("new registration", "id", reg.ID)
	http.Redirect(w, r, "/register?sent=1", http.StatusSeeOther)
}

type optionList struct {
	Key   string
	Label string
	Help  string
	Text  string
	Error string
}

type optionsForm struct {
	Lists     []optionList
	HasErrors bool
}

func optionLists(o models.RegisterOptions) []optionList {
	return []optionList{
		{Key: "property_types", Label: "Property types", Help: "e.g. Detached, Bungalow", Text: strings.Join(o.PropertyTypes, "\n")},
		{Key: "styles", Label: "Styles", Help: "e.g. Modern, Period", Text: strings.Join(o.Styles, "\n")},
		{Key: "ages", Label: "Age", Help: "e.g. New build, Victorian", Text: strings.Join(o.Ages, "\n")},
		{Key: "towns", Label: "Towns and villages", Help: "areas buyers can choose", Text: strings.Join(o.Towns, "\n")},
	}
}

// cleanOptions parses one option per line, dropping blanks and duplicates.
func cleanOptions(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range splitLines(text) {
		if utf8.RuneCountInString(line) > 80 {
			line = string([]rune(line)[:80])
		}
		key := strings.ToLower(line)
		if seen[key] || len(out) >= 100 {
			continue
		}
		seen[key] = true
		out = append(out, line)
	}
	return out
}

func (s *Server) adminRegistrations(w http.ResponseWriter, r *http.Request) {
	regs, err := s.store.ListRegistrations(0)
	if err != nil {
		s.serverError(w, err)
		return
	}
	props, err := s.store.ListProperties((*models.Property).Live)
	if err != nil {
		s.serverError(w, err)
		return
	}
	rows := make([]buyerRow, len(regs))
	for i, reg := range regs {
		rows[i].Registration = reg
		if reg.Consent {
			for _, p := range props {
				if reg.Matches(p) {
					rows[i].Matches = append(rows[i].Matches, p)
				}
			}
		}
	}
	s.renderAdmin(w, r, "admin_registrations", "registrations", "Registered buyers", rows, http.StatusOK)
}

func (s *Server) adminRegisterOptions(w http.ResponseWriter, r *http.Request) {
	opts, err := s.store.GetRegisterOptions()
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.renderAdmin(w, r, "admin_register_options", "options", "Register options", optionsForm{Lists: optionLists(opts)}, http.StatusOK)
}

func (s *Server) adminSaveRegisterOptions(w http.ResponseWriter, r *http.Request) {
	opts := models.RegisterOptions{
		PropertyTypes: cleanOptions(r.FormValue("property_types")),
		Styles:        cleanOptions(r.FormValue("styles")),
		Ages:          cleanOptions(r.FormValue("ages")),
		Towns:         cleanOptions(r.FormValue("towns")),
	}
	form := optionsForm{Lists: optionLists(opts)}
	for i := range form.Lists {
		l := &form.Lists[i]
		if l.Text == "" {
			l.Error = "Add at least one option."
			form.HasErrors = true
		}
	}
	if form.HasErrors {
		s.renderAdmin(w, r, "admin_register_options", "options", "Register options", form, http.StatusUnprocessableEntity)
		return
	}
	if err := s.store.SaveRegisterOptions(opts); err != nil {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/register-options?msg=options", http.StatusSeeOther)
}

type buyerRow struct {
	*models.Registration
	Matches []*models.Property
}

// matchingBuyers lists registered buyers who agreed to be contacted and whose preferences fit p.
func (s *Server) matchingBuyers(p *models.Property) ([]*models.Registration, error) {
	if !p.Live() {
		return nil, nil
	}
	regs, err := s.store.ListRegistrations(0)
	if err != nil {
		return nil, err
	}
	var out []*models.Registration
	for _, reg := range regs {
		if reg.Consent && reg.Matches(p) {
			out = append(out, reg)
		}
	}
	return out, nil
}
