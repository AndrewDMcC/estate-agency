package web

import (
	"embed"
	"html/template"
	"slices"
	"strings"
	"time"

	"github.com/AndrewDMcC/estate-agency/internal/models"
)

//go:embed templates static
var assets embed.FS

var pageLayouts = map[string]string{
	"home":                   "public",
	"properties":             "public",
	"property":               "public",
	"notfound":               "public",
	"register":               "public",
	"admin_registrations":    "admin",
	"admin_register_options": "admin",
	"admin_dashboard":        "admin",
	"admin_property_form":    "admin",
	"admin_enquiries":        "admin",
	"admin_settings":         "admin",
}

var templateFuncs = template.FuncMap{
	"price": models.FormatPrice,
	"lines": func(items []string) string { return strings.Join(items, "\n") },
	"paras": func(text string) []string {
		var out []string
		for _, p := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	},
	"date": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Local().Format("2 Jan 2006 15:04")
	},
	"inc":  func(i int) int { return i + 1 },
	"join": func(items []string) string { return strings.Join(items, ", ") },
	"has":  func(items []string, v string) bool { return slices.Contains(items, v) },
}

func parseTemplates() (map[string]*template.Template, error) {
	out := make(map[string]*template.Template, len(pageLayouts))
	for page, layout := range pageLayouts {
		t, err := template.New(page).Funcs(templateFuncs).ParseFS(assets,
			"templates/"+layout+"_layout.html",
			"templates/partials.html",
			"templates/"+page+".html",
		)
		if err != nil {
			return nil, err
		}
		out[page] = t
	}
	return out, nil
}
