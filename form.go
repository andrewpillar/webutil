package webutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/schema"
)

type sectionReadCloser struct {
	*io.SectionReader
}

// Form wraps the Fields and Validate methods for representing data that is
// being POSTed to an HTTP server for validation.
type Form interface {
	// Fields returns a map of all the fields in the form, and their string
	// values.
	Fields() map[string]string

	// Validate the given form and return any errors that occur. If validation
	// fails then it is expected for the returned error to be of the type
	// Errors.
	Validate() error
}

// File provides an implementation of the Form interface to validation file
// uploads via HTTP. It embeds the underlying multiepart.File type from the
// stdlib.
type File struct {
	multipart.File

	field string   // the name of the field in the form uploading the file
	size  int64    // the maximum size of the file, use 0 for no limit
	mimes []string // mimes to allow/disallow during upload
	allow bool     // whether or not we should allow/disallow the mimes

	Header *multipart.FileHeader // Header describes a file part of a multi part request.
	Type   string                // Type is the MIME of the file being uploaded.

	// Request is the current HTTP request through which the file is being
	// uploaded.
	Request *http.Request
}

var (
	_ multipart.File = (*sectionReadCloser)(nil)
	_ Form           = (*File)(nil)

	ErrFileTooLarge = errors.New("file is too large")
)

func humanSize(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	i := 0

	for ; n > 1024; i++ {
		n /= 1024
	}
	return fmt.Sprintf("%d %s", n, units[i])
}

// NewFile returns a new File for the given form field with the given maximum
// file size. If size is 0 then no limit is set on the size of a file that can
// be uploaded.
func NewFile(field string, size int64, r *http.Request) *File {
	return &File{
		field:   field,
		size:    size,
		Request: r,
	}
}

// Unmarshal will decode the contents of the given request into the given Form.
// If the request is of application/json then the entire body is decoded as
// JSON into the Form, otherwise this function assumes a typical form
// submission.
func Unmarshal(f Form, r *http.Request) error {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		err := json.NewDecoder(r.Body).Decode(f)

		// If an empty body validate anyway so we can get some substantial
		// error messages.
		if err == io.EOF {
			return f.Validate()
		}
		return err
	}

	if err := r.ParseForm(); err != nil {
		return err
	}

	dec := schema.NewDecoder()
	dec.IgnoreUnknownKeys(true)

	return dec.Decode(f, r.Form)
}

// UnmarshalAndValidate will unmarshal the given request into the given Form.
// and validate it. If an underlying schema.MultiError, or UnmarshalError
// occurs during unmarshalling then a validation attempt is still made.
func UnmarshalAndValidate(f Form, r *http.Request) error {
	errs := NewErrors()

	if err := Unmarshal(f, r); err != nil {
		switch v := err.(type) {
		case schema.EmptyFieldError:
			errs.Put(v.Key, ErrFieldRequired(v.Key))
		case schema.MultiError:
			for k, err := range v {
				errs.Put(k, err)
			}
		case UnmarshalError:
			errs.Put(v.Field, v.Err)
		default:
			return err
		}
	}

	if err := f.Validate(); err != nil {
		v, ok := err.(*Errors)

		if !ok {
			return err
		}
		errs.Merge(v)
	}
	return errs.Err()
}

// resolveFile will parse the request body and attempt to extract a file that
// was sent in it. If the Content-Type of the request is of multipart/form-data
// then FormFile will be called on the request, otherwise the entire request
// body is treated as the file being uploaded. If the file found in the request
// exceeds the configured size then ErrFileTooLarge is returned.
func (f *File) resolveFile() error {
	if strings.HasPrefix(f.Request.Header.Get("Content-Type"), "multipart/form-data") {
		file, header, err := f.Request.FormFile(f.field)

		if err != nil {
			return err
		}

		if f.size > 0 {
			if header.Size > f.size {
				return ErrFileTooLarge
			}
		}

		f.File = file
		f.Header = header
		return nil
	}

	var buf bytes.Buffer

	if _, err := io.Copy(&buf, f.Request.Body); err != nil {
		return err
	}

	if f.size > 0 {
		if int64(buf.Len()) > f.size {
			return ErrFileTooLarge
		}
	}

	b := buf.Bytes()

	f.File = sectionReadCloser{
		SectionReader: io.NewSectionReader(bytes.NewReader(b), 0, int64(len(b))),
	}
	return nil
}

// Allow specifies a list of mimes we want to allow during file upload.
// This will rever any preceding calls to Disallowed.
func (f *File) Allow(mimes ...string) {
	f.mimes = mimes
	f.allow = true
}

// Disallow specifies a list of mimes we want to allow during file upload.
// This will revert any preceding calls to Allowed.
func (f *File) Disallow(mimes ...string) {
	f.mimes = mimes
	f.allow = false
}

// Fields will always return nil.
func (*File) Fields() map[string]string { return nil }

// Remove the file if it exists on disk. If the underlying multipart.File is
// not of *os.File, then this does nothing.
func (f *File) Remove() error {
	if v, ok := f.File.(*os.File); ok {
		return os.RemoveAll(v.Name())
	}
	return nil
}

// Validate will check the size of the file being uploaded, and set the Type
// of the file. If any mimes have been set then these will be checked to
// determine of the Type of the file is allowed or disallowed. If the request
// the file was sent over is anything over than multipart/form-data, then the
// entire request body is treated as the file contents itself.
func (f *File) Validate() error {
	errs := NewErrors()

	if err := f.resolveFile(); err != nil {
		if err == ErrFileTooLarge {
			errs.Put(f.field, fmt.Errorf("%s cannot be bigger than %s", f.field, humanSize(f.size)))
		}

		if strings.Contains(err.Error(), "no such file") {
			errs.Put(f.field, ErrFieldRequired(f.field))
		}
		return errs.Err()
	}

	header := make([]byte, 512)

	if _, err := f.Read(header); err != nil {
		if err == io.EOF {
			errs.Put(f.field, ErrFieldRequired(f.field))
		}
		return errs
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	f.Type = http.DetectContentType(header)

	mimes := strings.Join(f.mimes, ", ")

	msgs := map[bool]string{
		true:  "must be one of",
		false: "cannot be one of",
	}

	for _, mime := range f.mimes {
		if (f.Type == mime) != f.allow {
			errs.Put(f.field, fmt.Errorf("%s %s %s", f.field, msgs[f.allow], mimes))
		}
	}
	return errs.Err()
}

func (sectionReadCloser) Close() error { return nil }
