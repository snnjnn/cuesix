package schema

import (
	"errors"
	"html/template"
	"io"
	"net/http"
	"sort"
	"sync"
)

const echoMaxBodyBytes = 64 * 1024
const echoHeaderValueMaxLen = 80

type headerEntry struct {
	Name   string
	Values []string
}

type echoPageData struct {
	Method        string
	Path          string
	Headers       []headerEntry
	Body          string
	BodyTruncated bool
	BodyError     string
}

var (
	echoTemplateOnce sync.Once
	echoTemplate     *template.Template
	echoTemplateErr  error
)

func loadEchoTemplate() (*template.Template, error) {
	echoTemplateOnce.Do(func() {
		funcs := template.FuncMap{
			"truncateHeaderValue": truncateHeaderValue,
			"isHeaderTruncated":   isHeaderTruncated,
		}
		echoTemplate, echoTemplateErr = template.New("echo.html").Funcs(funcs).ParseFS(playgroundFS, "app/dist/echo.html")
	})
	return echoTemplate, echoTemplateErr
}

func EchoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, truncated, bodyErr := readEchoBody(w, r)
		data := echoPageData{
			Method:        r.Method,
			Path:          r.URL.String(),
			Headers:       collectHeaders(r.Header),
			Body:          body,
			BodyTruncated: truncated,
		}
		if bodyErr != nil {
			data.BodyError = bodyErr.Error()
		}

		tmpl, err := loadEchoTemplate()
		if err != nil {
			http.Error(w, "failed to load echo template", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "failed to render echo page", http.StatusInternalServerError)
			return
		}
	})
}

func readEchoBody(w http.ResponseWriter, r *http.Request) (string, bool, error) {
	if r.Body == nil {
		return "", false, nil
	}

	limited := http.MaxBytesReader(w, r.Body, echoMaxBodyBytes)
	defer func() {
		_ = limited.Close()
	}()

	bodyBytes, err := io.ReadAll(limited)
	if err == nil {
		return string(bodyBytes), false, nil
	}

	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return string(bodyBytes), true, nil
	}

	return string(bodyBytes), false, err
}

func collectHeaders(header http.Header) []headerEntry {
	headers := make([]headerEntry, 0, len(header))
	for name, values := range header {
		headers = append(headers, headerEntry{
			Name:   name,
			Values: values,
		})
	}

	sort.Slice(headers, func(i, j int) bool {
		return headers[i].Name < headers[j].Name
	})

	return headers
}

func truncateHeaderValue(value string) string {
	runes := []rune(value)
	if len(runes) <= echoHeaderValueMaxLen {
		return value
	}
	return string(runes[:echoHeaderValueMaxLen-1]) + "…"
}

func isHeaderTruncated(value string) bool {
	return len([]rune(value)) > echoHeaderValueMaxLen
}
