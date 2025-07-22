package httpserver

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/paragor/sharefile/internal/httpserver/htmxtemplates"
)

var templates *template.Template

func init() {
	templates = template.New("")
	templates = must(templates.ParseFS(htmxtemplates.Components, "components/*.html"))
	templates = must(templates.ParseFS(htmxtemplates.Pages, "pages/*.html"))
}

func renderHtmx(template string, data any) (*bytes.Buffer, error) {
	buffer := bytes.NewBuffer(nil)
	if err := templates.ExecuteTemplate(buffer, template, data); err != nil {
		return nil, err
	}

	return buffer, nil
}

func writeHtmx(writer http.ResponseWriter, request *http.Request, template string, data any, status int) {
	buffer, err := renderHtmx(template, data)
	if err != nil {
		httpError(request.Context(), writer, "error on render", err, http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = writer.Write(buffer.Bytes())
}

func must[T any](result T, err error) T {
	if err != nil {
		panic(err)
	}
	return result
}
