package webutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/gorilla/schema"
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

// Text sets the Content-Type of the given ResponseWriter to text/plain, and
// writes the given content with the given status code to the writer. This will
// also se the Content-Length header to the len of content.
func Text(w http.ResponseWriter, content string, status int) {
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(content))
}

// HTML sets the Content-Type of the given ResponseWriter to text/html, and
// writes the given content with the given status code to the writer. This will
// also set the Content-Length header to the len of content.
func HTML(w http.ResponseWriter, content string, status int) {
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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

	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

const (
	formKey = "form_fields"
	errsKey = "form_errors"
)

// FormErrors returns the Errors that has been flashed to the given session
// under the "form_errors" key. If the key does not exist, then an empty Errors
// is returned instead.
func FormErrors(sess *sessions.Session) ValidationErrors {
	val := sess.Flashes(errsKey)

	if val == nil {
		return make(ValidationErrors)
	}

	err, ok := val[0].(ValidationErrors)

	if !ok {
		return make(ValidationErrors)
	}
	return err
}

// FormField returns the map of form fields that has been flashed to the given
// session under the "form_fields" key. If the key does not exist, then an
// empty map is returned instead.
func FormFields(sess *sessions.Session) map[string]string {
	val := sess.Flashes(formKey)

	if val == nil {
		return map[string]string{}
	}

	m, ok := val[0].(map[string]string)

	if !ok {
		return map[string]string{}
	}
	return m
}

// ValidationErrors records any validation errors that may have occurred. Each
// error is kept beneath the field for which the error occurred.
type ValidationErrors map[string][]string

func (e ValidationErrors) Err() error {
	if len(e) == 0 {
		return nil
	}
	return e
}

// Add adds the given error for the given field.
func (e ValidationErrors) Add(field string, err error) {
	if converr, ok := err.(schema.ConversionError); ok {
		err = converr.Err
	}
	e[field] = append(e[field], err.Error())
}

func (e ValidationErrors) merge(errs ValidationErrors) {
	for field, errs2 := range errs {
		e[field] = append(e[field], errs2...)
	}
}

// First returns the first error message for the given field if any.
func (e ValidationErrors) First(field string) string {
	errs, ok := e[field]

	if !ok {
		return ""
	}
	return errs[0]
}

// Error returns the string representation of the current set of errors. It will
// be formatted like so,
//
//	field:
//	    err
//	    err
func (e ValidationErrors) Error() string {
	var buf strings.Builder

	for key, errs := range e {
		buf.WriteString(key + ":\n")

		for _, err := range errs {
			buf.WriteString("    " + err + "\n")
		}
	}
	return buf.String()
}

var (
	ErrFieldRequired = errors.New("field required")
	ErrFieldExists   = errors.New("already exists")
)

// FieldError captures an error and the field name that caused it.
type FieldError struct {
	Name string
	Err  error
}

func (e *FieldError) Error() string {
	return e.Name + " " + e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *FieldError) Unwrap() error {
	return e.Err
}

// Form is the interface used for unmarhsalling and validating form data sent
// in an HTTP request.
type Form interface {
	// Fields returns a map of all the form's underlying values.
	Fields() map[string]string

	// Validate validates the form. This should return an error type of
	// ValidationErrors should validation fail.
	Validate(ctx context.Context) error
}

func unmarshalRequest(f Form, r *http.Request) error {
	typ := r.Header.Get("Content-Type")

	if strings.HasPrefix(typ, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(f); err != nil {
			if !errors.Is(err, io.EOF) {
				return err
			}
		}
		return nil
	}

	if r.Form == nil {
		if err := r.ParseForm(); err != nil {
			return err
		}
	}

	dec := schema.NewDecoder()
	dec.IgnoreUnknownKeys(true)

	if err := dec.Decode(f, r.Form); err != nil {
		return err
	}
	return nil
}

// UnmarshalForm parses the request into the given Form. If any errors occur
// during unmarshalling, then these will be returned via the ValidationErrors
// type.
func UnmarshalForm(f Form, r *http.Request) error {
	if err := unmarshalRequest(f, r); err != nil {
		errs := make(ValidationErrors)

		switch v := err.(type) {
		case schema.EmptyFieldError:
			errs.Add(v.Key, ErrFieldRequired)
		case schema.MultiError:
			for field, err := range v {
				errs.Add(field, err)
			}
		case *json.UnmarshalFieldError:
			errs.Add(string(v.Field.Tag), err)
		case *json.UnmarshalTypeError:
			val := reflect.ValueOf(f)

			if el := val.Elem(); el.Kind() == reflect.Struct {
				typ := el.Type()

				if field, ok := typ.FieldByName(v.Field); ok {
					if tag, ok := field.Tag.Lookup("json"); ok {
						v.Field = tag
					}
				}
			}
			errs.Add(v.Field, errors.New("cannot unmarshal "+v.Value+" to "+v.Type.String()))
		case *FieldError:
			errs.Add(v.Name, v.Err)
		default:
			return err
		}
		return errs.Err()
	}
	return nil
}

// UnmarshalFormAndValidate parses the request into the given Form. If
// unmmarshalling succeeds, then the Form is validated via the Validate method.
func UnmarshalFormAndValidate(f Form, r *http.Request) error {
	var errs ValidationErrors

	if err := UnmarshalForm(f, r); err != nil {
		if v, ok := err.(ValidationErrors); ok {
			errs = v
			goto validate
		}
		return err
	}

validate:
	if err := f.Validate(r.Context()); err != nil {
		if v, ok := err.(ValidationErrors); ok && errs != nil {
			errs.merge(v)
		}
		return err
	}
	return nil
}

// File is used for unmarshalling files from requests.
type File struct {
	// The underlying file that was uploaded.
	multipart.File

	// Header is the header of the file being uploaded.
	Header *multipart.FileHeader

	// Type is the MIME type of the file, this is set during the unmarshalling
	// of the file by sniffing the first 512 bytes of the file.
	Type string
}

// Remove will remove the underlying file if it was written to disk during the
// upload. This will typically only be done if the file was too large to store
// in memory.
func (f *File) Remove() error {
	if v, ok := f.File.(*os.File); ok {
		return os.RemoveAll(v.Name())
	}
	return nil
}

func (f *File) HasFile() bool { return f.File != nil }

func unmarshalMimeType(rs io.ReadSeeker) (string, error) {
	hdr := make([]byte, 512)

	if _, err := rs.Read(hdr); err != nil {
		return "", err
	}

	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return http.DetectContentType(hdr), nil
}

// defaultMaxMemory is the maximum amount of memory to use when uploading a
// file. If this is exceeded, then the file is written to a temporary file on
// disk.
var defaultMaxMemory int64 = 32 << 20

// UnmarshalFiles parses the request for every file sent with it. The request
// must have the Content-Type of multipart/form-data, otherwise an error is
// returned. A boolean is returned for whether or not any files were sent in the
// request.
func UnmarshalFiles(field string, r *http.Request) ([]*File, bool, error) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		return nil, false, errors.New("invalid request type")
	}

	if err := r.ParseMultipartForm(defaultMaxMemory); err != nil {
		return nil, false, err
	}

	hdrs, ok := r.MultipartForm.File[field]

	if !ok {
		return nil, false, nil
	}

	files := make([]*File, 0, len(hdrs))

	for _, hdr := range hdrs {
		f, err := hdr.Open()

		if err != nil {
			return nil, false, err
		}

		typ, err := unmarshalMimeType(f)

		if err != nil {
			return nil, false, err
		}

		files = append(files, &File{
			File:   f,
			Header: hdr,
			Type:   typ,
		})
	}
	return files, true, nil
}

type sectionReadCloser struct {
	*io.SectionReader
}

var _ multipart.File = (*sectionReadCloser)(nil)

func (sectionReadCloser) Close() error { return nil }

// UnmarshalFile parses the request for a file sent with it. If the request has
// the Content-Type of multipart/form-data, then the file will be taken from
// the multipart form via the given field, otherwise it will be taken from the
// request body. A boolean is returned for whether or not a file was sent in
// the request.
func UnmarshalFile(field string, r *http.Request) (*File, bool, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		files, ok, err := UnmarshalFiles(field, r)

		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, nil
		}
		return files[0], true, nil
	}

	var (
		buf bytes.Buffer
		f   multipart.File
	)

	n, err := io.CopyN(&buf, r.Body, defaultMaxMemory+1)

	if err != nil {
		if !errors.Is(err, io.EOF) {
			return nil, false, err
		}
	}

	if n == 0 {
		return nil, true, nil
	}

	f = sectionReadCloser{
		SectionReader: io.NewSectionReader(bytes.NewReader(buf.Bytes()), 0, n),
	}

	if n > defaultMaxMemory {
		tmp, err := os.CreateTemp("", "webutil-file-")

		if err != nil {
			return nil, false, err
		}

		n, err = io.Copy(tmp, io.MultiReader(&buf, r.Body))

		if err != nil {
			os.Remove(tmp.Name())
			return nil, false, err
		}

		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			os.Remove(tmp.Name())
			return nil, false, err
		}
		f = tmp
	}

	typ, err := unmarshalMimeType(f)

	if err != nil {
		return nil, false, err
	}

	return &File{
		File: f,
		Header: &multipart.FileHeader{
			Header: make(textproto.MIMEHeader),
			Size:   n,
		},
		Type: typ,
	}, true, nil
}

// UnmarshalFormWithFile parses the request for a file sent with it. If the
// request has the Content-Type of multipart/form-data, then the file will be
// taken from the multipart form via the given field. This will then unmarshal
// the rest of the request data into the given Form. If file in the request was
// sent in the request body, then the URL query parameters are unmarshalled
// into the given Form. A boolean is returned for whether or not a file was a
// file in the request.
func UnmarshalFormWithFile(f Form, field string, r *http.Request) (*File, bool, error) {
	file, ok, err := UnmarshalFile(field, r)

	if err != nil {
		return nil, false, err
	}

	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Form = r.URL.Query()
	}

	if err := UnmarshalForm(f, r); err != nil {
		return nil, false, err
	}
	return file, ok, nil
}

// UnmarshalFormWithFiles parses the request for every file sent with it. The
// request must have the Content-Type of multipart/form-data, otherwise an
// error is returned. This will then unmarshal the rest of the request data into
// the given Form. A boolean is returned for whether or not any files were sent
// in the request.
func UnmarshalFormWithFiles(f Form, field string, r *http.Request) ([]*File, bool, error) {
	files, ok, err := UnmarshalFiles(field, r)

	if err != nil {
		return nil, false, err
	}

	if err := UnmarshalForm(f, r); err != nil {
		return nil, false, err
	}
	return files, ok, nil
}
