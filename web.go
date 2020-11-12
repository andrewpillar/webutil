package webutil

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/sessions"
)

// BaseAddress will return the HTTP address for the given Request. This will
// return the Scheme of the current Request (http, or https), concatenated with
// the host. If the X-Forwarded-Proto, and X-Forwarded-Host headers are present
// in the Request, then they will be used for the Scheme and Host respectively.
func BaseAddress(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	host := r.Header.Get("X-Forwarded-Host")

	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

// BasePath returns the last element of the given path. This will split the
// path using the "/" spearator. If the path is empty BasePath returns "/".
func BasePath(path string) string {
	if path == "" || path == "/" {
		return "/"
	}
	parts := strings.Split(path, "/")

	base := strings.TrimSuffix(parts[len(parts)-1], "/")

	if base == "" {
		return "/"
	}
	return base
}

// FormErrors returns the Errors that has been flashed to the given session
// under the "form_errors" key. If the key does not exist, then an empty Errors
// is returned instead.
func FormErrors(sess *sessions.Session) *Errors {
	val := sess.Flashes("form_errors")

	if val == nil {
		return NewErrors()
	}

	err, ok := val[0].(*Errors)

	if !ok {
		return NewErrors()
	}
	return err
}

// FormField returns the map of form fields that has been flashed to the given
// session under the "form_fields" key. If the key does not exist, then an
// empty map is returned instead.
func FormFields(sess *sessions.Session) map[string]string {
	val := sess.Flashes("form_fields")

	if val == nil {
		return map[string]string{}
	}

	m, ok := val[0].(map[string]string)

	if !ok {
		return map[string]string{}
	}
	return m
}

// FlashFormWithErrors flashes the given Form and Errors to the given session
// under the "form_fields" and "form_errors" keys respectively.
func FlashFormWithErrors(sess *sessions.Session, f Form, errs *Errors) {
	sess.AddFlash(f.Fields(), "form_fields")
	sess.AddFlash(errs, "form_errors")
}

// HTML sets the Content-Type of the given ResponseWriter to text/html, and
// writes the given content with the given status code to the writer. This will
// also set the Content-Length header to the len of content.
func HTML(w http.ResponseWriter, content string, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
	w.WriteHeader(status)
	w.Write([]byte(content))
}

// JSON sets the Content-Type of the given ResponseWriter to application/json,
// and encodes the given interface to JSON to the given writer, with the given
// status code. This will also set the Content-Length header to the len of the
// JSON encoded data.
func JSON(w http.ResponseWriter, data interface{}, status int) {
	var buf bytes.Buffer

	json.NewEncoder(&buf).Encode(data)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.FormatInt(int64(buf.Len()), 10))
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

// Text sets the Content-Type of the given ResponseWriter to text/plain, and
// writes the given content with the given status code to the writer. This will
// also se the Content-Length header to the len of content.
func Text(w http.ResponseWriter, content string, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
	w.WriteHeader(status)
	w.Write([]byte(content))
}
