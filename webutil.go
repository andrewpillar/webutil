package webutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

// HTML sets the Content-Type of the given ResponseWriter to text/html, and
// writes the given content with the given status code to the writer. This will
// also set the Content-Length header to the len of content.
func HTML(w http.ResponseWriter, content string, status int) {
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(content))
}

var (
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

// FlashFormWithErrors flashes the given Form and Errors to the given session
// under the "form_fields" and "form_errors" keys respectively.
func FlashFormWithErrors(sess *sessions.Session, f Form, errs ValidationErrors) {
	sess.AddFlash(f.Fields(), formKey)
	sess.AddFlash(errs, errsKey)
}

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

// JSON sets the Content-Type of the given ResponseWriter to application/json,
// and encodes the given interface to JSON to the given writer, with the given
// status code. This will also set the Content-Length header to the len of the
// JSON encoded data.
func JSON(w http.ResponseWriter, data interface{}, status int) {
	var buf bytes.Buffer

	json.NewEncoder(&buf).Encode(data)

	w.Header().Set("Content-Length", strconv.FormatInt(int64(buf.Len()), 10))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

// Text sets the Content-Type of the given ResponseWriter to text/plain, and
// writes the given content with the given status code to the writer. This will
// also se the Content-Length header to the len of content.
func Text(w http.ResponseWriter, content string, status int) {
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(content)), 10))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(content))
}

type Form interface {
	// Fields returns a map of all the form's underlying values.
	Fields() map[string]string
}

// FormUnmarshaler is used for unmarshalling forms from requests.
type FormUnmarshaler struct {
	Form    Form            // The Form to decode the values to, must be a pointer.
	Decoder *schema.Decoder // The decoder to use for decoding form data.
}

// UnmarshalRequest will unmarshal the given request to the underlying form.
// If the request has the Content-Type header set to "application/json" then
// the request body is decoded as JSON.
func (f FormUnmarshaler) UnmarshalRequest(r *http.Request) error {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(r.Body).Decode(f.Form); err != nil {
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

	if err := f.Decoder.Decode(f.Form, r.Form); err != nil {
		return err
	}
	return nil
}

// UnmarshalForm unmarshals the given request into the given form. This will
// return any unmarshalling errors for individual fields in a ValidationErrors
// type.
func UnmarshalForm(f Form, r *http.Request) error {
	u := FormUnmarshaler{
		Form:    f,
		Decoder: schema.NewDecoder(),
	}

	u.Decoder.IgnoreUnknownKeys(true)

	if err := u.UnmarshalRequest(r); err != nil {
		verrs := NewValidationErrors()

		switch v := err.(type) {
		case schema.EmptyFieldError:
			verrs.Add(v.Key, ErrFieldRequired(v.Key))
		case schema.MultiError:
			for field, err := range v {
				verrs.Add(field, err)
			}
		case *json.UnmarshalFieldError:
			verrs.Add(string(v.Field.Tag), err)
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
			verrs.Add(v.Field, errors.New("cannot unmarshal "+v.Value+" to "+v.Type.String()))
		case UnmarshalError:
			verrs.Add(v.Field, v.Err)
		default:
			return err
		}
		return verrs
	}
	return nil
}

// File is used for unmarshalling files from requests.
type File struct {
	multipart.File // The underlying file that was uploaded.

	// Header is the header of the file being uploaded.
	Header *multipart.FileHeader

	// Type is the MIME type of the file, this is set during the unmarshalling
	// of the file by sniffing the first 512 bytes of the file.
	Type string

	// Field is the form field to unmarshal the file from, if the file is being
	// uploaded as part of a "multipart/form-data" request.
	Field  string
}

// UnmarshalFormWithFile will unmarshal a file from the given request, then it
// it will unmarshal the rest of the request data into the given form. If the
// file is sent as the request body itself, then the URL query parameters will
// be used to unmarshal the rest of the form data from.
func UnmarshalFormWithFile(f Form, file *File, r *http.Request) error {
	if err := file.UnmarshalRequest(r); err != nil {
		return err
	}

	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Form = r.URL.Query()
	}

	if err := UnmarshalForm(f, r); err != nil {
		return err
	}
	return nil
}

type sectionReadCloser struct {
	*io.SectionReader
}

var _ multipart.File = (*sectionReadCloser)(nil)

func (sectionReadCloser) Close() error { return nil }

func (f *File) unmarshalType() error {
	hdr := make([]byte, 512)

	if _, err := f.Read(hdr); err != nil {
		return err
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	f.Type = http.DetectContentType(hdr)
	return nil
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

// UnmarshalRequest will unmarshal a file from the given request. If the file
// was sent as the request body, then the header of the file will only be
// partially populated with the size of the file, and nothing else.
func (f *File) UnmarshalRequest(r *http.Request) error {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		var err error

		f.File, f.Header, err = r.FormFile(f.Field)

		if err != nil {
			if !errors.Is(err, http.ErrMissingFile) {
				return err
			}
			return nil
		}

		if err := f.unmarshalType(); err != nil {
			return err
		}
		return nil
	}

	var buf bytes.Buffer

	if _, err := io.Copy(&buf, r.Body); err != nil {
		return err
	}

	size := int64(buf.Len())

	if size == 0 {
		return nil
	}

	f.File = sectionReadCloser{
		SectionReader: io.NewSectionReader(bytes.NewReader(buf.Bytes()), 0, size),
	}
	f.Header = &multipart.FileHeader{
		Header: make(textproto.MIMEHeader),
		Size:   size,
	}

	if err := f.unmarshalType(); err != nil {
		if !errors.Is(err, io.EOF) {
			return err
		}
	}
	return nil
}

// FileValidator is a Validator implementation for validating file uploads.
type FileValidator struct {
	*File // The uploaded file.

	// Size is the maximum size of a file. Set to 0 for no limit.
	Size int64

	// Mimes is a list of MIME types to allow/disallow during uploading.
	Mimes []string

	// MimesAllowed delineates whether or not the above slice of MIME types
	// should be allowed/disallowed. Set to false to disallow, set to true to
	// allow.
	MimesAllowed bool
}

func humanSize(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	i := 0

	for ; n > 1024; i++ {
		n /= 1024
	}
	return fmt.Sprintf("%d %s", n, units[i])
}

// HasFile will check to see if a file has been uploaded.
func (v *FileValidator) HasFile() bool { return v.File.File != nil }

// Validate will check if a file has been validated, along with whether or not
// it is within the size limit, and of the allowed MIME types.
func (v *FileValidator) Validate(errs ValidationErrors) {
	if !v.HasFile() {
		errs.Add(v.Field, ErrFieldRequired(v.Field))
		return
	}

	if v.Size > 0 {
		if v.Header.Size > v.Size {
			errs.Add(v.Field, errors.New(v.Field+" cannot be bigger than "+humanSize(v.Size)))
		}
	}

	mimes := strings.Join(v.Mimes, ", ")

	var err error

	for _, mime := range v.Mimes {
		if (v.Type == mime) != v.MimesAllowed {
			if v.MimesAllowed {
				err = errors.New(v.Field + " must be one of " + mimes)
			} else {
				err = errors.New(v.Field + " cannot be one of " + mimes)
			}
			errs.Add(v.Field, err)
		}
	}
}

type UnmarshalError struct {
	Field string
	Err   error
}

func (e UnmarshalError) Error() string {
	return "failed to unmarshal " + e.Field + ": " + e.Err.Error()
}

// ValidationErrors records any validation errors that may have occurred. Each
// error is kept beneath the field for which the error occurred.
type ValidationErrors map[string][]string

// NewValidationErrors returns an empty ValidationErrors.
func NewValidationErrors() ValidationErrors { return make(ValidationErrors) }

func ErrFieldRequired(field string) error {
	return errors.New(field + " field is required")
}

func ErrFieldExists(field string) error {
	return errors.New(field + " already exists")
}

// Add adds the given error for the given key.
func (e ValidationErrors) Add(key string, err error) {
	if cerr, ok := err.(schema.ConversionError); ok {
		err = cerr.Err
	}
	e[key] = append(e[key], err.Error())
}

// Merge merges the given set of errors into the current one.
func (e ValidationErrors) Merge(verrs ValidationErrors) {
	for key, errs := range verrs {
		e[key] = append(e[key], errs...)
	}
}

// Error returns the string representation of the current set of errors. It will
// be formatted like so,
//
//   field:
//       err
//       err
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

// First returns the first error for the given key if any.
func (e ValidationErrors) First(key string) string {
	errs, ok := e[key]

	if !ok {
		return ""
	}
	return errs[0]
}

type Validator interface {
	// Validate performs validation on a set of data. Each error that occurs
	// should be added to the given set of errors.
	Validate(errs ValidationErrors)
}

// Validate will call Validate on the given Validator. If the given Validator
// fails, then the returned error will be of type ValidationErrors, otherwise
// it will be nil.
func Validate(v Validator) error {
	errs := NewValidationErrors()

	v.Validate(errs)

	if len(errs) > 0 {
		return errs
	}
	return nil
}
