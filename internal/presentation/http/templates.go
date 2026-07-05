package http

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templatesFS embed.FS

// MustLoadTemplates parses all embedded HTML templates or panics.
func MustLoadTemplates() *template.Template {
	return template.Must(template.ParseFS(templatesFS, "templates/*.html"))
}
