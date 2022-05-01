package webutil

import (
	"bytes"
	"crypto/tls"
	"image"
	"image/jpeg"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func Test_BaseAddress(t *testing.T) {
	tests := []struct {
		request  *http.Request
		expected string
	}{
		{
			&http.Request{
				Header: http.Header{
					"X-Forwarded-Proto": []string{"https"},
					"X-Forwarded-Host":  []string{"example.com"},
				},
				URL:    &url.URL{
					Scheme: "http",
					Host:   "localhost:8080",
					Path:   "/api/files/1/download",
				},
			},
			"https://example.com",
		},
		{

			&http.Request{
				Header: http.Header{
					"X-Forwarded-Host":  []string{"example.com"},
				},
				URL:    &url.URL{
					Scheme: "http",
					Host:   "localhost:8080",
					Path:   "/api/files/1/download",
				},
				TLS: &tls.ConnectionState{},
			},
			"https://example.com",
		},
		{

			&http.Request{
				URL:    &url.URL{
					Scheme: "http",
					Host:   "localhost:8080",
					Path:   "/api/files/1/download",
				},
				Host: "localhost:8080",
			},
			"http://localhost:8080",
		},
	}

	for i, test := range tests {
		if addr := BaseAddress(test.request); addr != test.expected {
			t.Errorf("tests[%d] - unexpected base address, expected=%q, got=%q\n", i, test.expected, addr)
		}
	}
}

func Test_BasePath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"", "/"},
		{"/", "/"},
		{"/////", "/"},
		{"\\\\///", "/"},
		{"/api/files/1/download", "download"},
	}

	for i, test := range tests {
		if path := BasePath(test.path); path != test.expected {
			t.Errorf("tests[%d] - unexpected base path, expected=%q, got=%q\n", i, test.expected, path)
		}
	}
}

type Post struct {
	Title string `json:"title" schema:"title"`
	Body  string `json:"body"  schame:"body"`
}

var _ Form = (*Post)(nil)

func (f Post) Fields() map[string]string {
	return map[string]string{
		"title": f.Title,
		"body":  f.Body,
	}
}

type PostValidator struct {
	Form Post
}

var _ Validator = (*PostValidator)(nil)

func (v PostValidator) Validate(errs ValidationErrors) {
	if v.Form.Title == "" {
		errs.Add("title", ErrFieldRequired("Title"))
	}
	if v.Form.Body == "" {
		errs.Add("body", ErrFieldRequired("Body"))
	}
}

func Test_Form(t *testing.T) {
	tests := []struct {
		form      Post
		req       *http.Request
		shoulderr bool
		errs      []string
	}{
		{
			Post{},
			&http.Request{
				Form: url.Values{
					"title": []string{"my post"},
					"body":  []string{"body"},
				},
			},
			false,
			nil,
		},
		{
			Post{},
			&http.Request{
				Header: http.Header{
					"Content-Type": []string{"application/json; charset=utf-8"},
				},
				Body: io.NopCloser(strings.NewReader(`{"title": "my post", "body": "body"}`)),
			},
			false,
			nil,
		},
		{
			Post{},
			&http.Request{},
			true,
			[]string{"body", "title"},
		},
		{
			Post{},
			&http.Request{
				Header: http.Header{
					"Content-Type": []string{"application/json; charset=utf-8"},
				},
				Body: io.NopCloser(strings.NewReader(`{"title": -1, "body": [1, 2, 3]}`)),
			},
			true,
			[]string{"body", "title"},
		},
	}

	for i, test := range tests {
		verrs := make(ValidationErrors)

		if err := UnmarshalForm(&test.form, test.req); err != nil {
			if verrs0, ok := err.(ValidationErrors); ok {
				if test.shoulderr {
					verrs = verrs0
					goto validate
				}
			}
			t.Errorf("tests[%d] - unexpected UnmarshalForm error: %s\n", i, err)
			continue
		}

validate:
		v := PostValidator{
			Form: test.form,
		}

		if err := Validate(v); err != nil {
			if !test.shoulderr {
				t.Errorf("tests[%d] - unexpected Validate error: %s", i, err)
				continue
			}

			err.(ValidationErrors).Merge(verrs)
			verrs = err.(ValidationErrors)

			if len(verrs) != len(test.errs) {
				t.Errorf("tests[%d] - unexpected error count, expected=%d, got=%d\n", i, len(test.errs), len(verrs))
				continue
			}

			for _, field := range test.errs {
				if _, ok := verrs[field]; !ok {
					t.Errorf("tests[%d] - expected field %q in errors\n", i, field)
				}
			}
		}
	}
}

func randImage(t *testing.T) *bytes.Buffer {
	var buf bytes.Buffer

	img := image.NewAlpha(
		image.Rect(rand.Intn(255), rand.Intn(255), rand.Intn(255), rand.Intn(255)),
	)

	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 1}); err != nil {
		t.Fatalf("unexpected jpeg.Encode error: %s\n", err)
	}
	return &buf
}

func multipartRequest(t *testing.T) *http.Request {
	pr, pw := io.Pipe()

	mpw := multipart.NewWriter(pw)

	go func() {
		defer mpw.Close()

		w, err := mpw.CreateFormFile("avatar", "dog-in-hat-and-scarf.jpg")

		if err != nil {
			t.Fatalf("unexpected multipart.Writer.CreateFormFile error: %s\n", err)
		}
		io.Copy(w, randImage(t))
	}()

	r := httptest.NewRequest("POST", "/upload", pr)
	r.Header.Add("Content-Type", mpw.FormDataContentType())

	return r
}

func Test_FileLimit(t *testing.T) {
	r := multipartRequest(t)

	f := &File{
		Field: "avatar",
	}

	u := Upload{}

	if err := UnmarshalFormWithFile(&u, f, r); err != nil {
		t.Fatalf("unexpected UnmarshalFormWithFile error: %s\n", err)
	}

	v := FileValidator{
		File: f,
		Size: 1,
	}

	err := Validate(&v)

	if err == nil {
		t.Fatalf("exepcted Validate to error, it did not\n")
	}

	verrs := err.(ValidationErrors)

	expected := "avatar cannot be bigger than 1 B"

	if msg := verrs.First("avatar"); msg != expected {
		t.Fatalf("unexpected error, expected=%q, got=%q\n", expected, msg)
	}
}

func Test_FileAllow(t *testing.T) {
	r := multipartRequest(t)

	f := &File{
		Field: "avatar",
	}

	u := Upload{}

	if err := UnmarshalFormWithFile(&u, f, r); err != nil {
		t.Fatalf("unexpected UnmarshalFormWithFile error: %s\n", err)
	}

	v := FileValidator{
		File:         f,
		Mimes:        []string{"image/jpeg"},
		MimesAllowed: true,
	}

	if err := Validate(&v); err != nil {
		t.Fatalf("unexpected Validate error: %s\n", err)
	}
}

func Test_FileDisallow(t *testing.T) {
	r := multipartRequest(t)

	f := &File{
		Field: "avatar",
	}

	u := Upload{}

	if err := UnmarshalFormWithFile(&u, f, r); err != nil {
		t.Fatalf("unexpected UnmarshalFormWithFile error: %s\n", err)
	}

	v := FileValidator{
		File:  f,
		Mimes: []string{"image/jpeg"},
	}

	if err := Validate(&v); err == nil {
		t.Fatalf("expected Validate to error, it did not\n")
	}
}

type Upload struct {
	File *File
	Name string
}

var _ Form = (*Upload)(nil)

func (f Upload) Fields() map[string]string {
	return map[string]string{"name": f.Name}
}

type UploadValidator struct {
	File *FileValidator
	Form Upload
}

var _ Validator = (*UploadValidator)(nil)

func (v UploadValidator) Validate(errs ValidationErrors) {
	if v.Form.Name == "" {
		errs.Add("name", ErrFieldRequired("Name"))
	}
}

func Test_FileBody(t *testing.T) {
	url, _ := url.Parse("https://api.example.com/upload?name=my-file")

	r := &http.Request{
		Header: http.Header{
			"Content-Type": []string{"image/jpeg"},
		},
		URL:  url,
		Body: io.NopCloser(randImage(t)),
	}

	f := Upload{
		File: &File{},
	}

	if err := UnmarshalFormWithFile(&f, f.File, r); err != nil {
		t.Fatalf("unexpected UnmarshalFormWithFile error: %s\n", err)
	}

	v := UploadValidator{
		File: &FileValidator{
			File: f.File,
		},
		Form: f,
	}

	if err := Validate(v); err != nil {
		t.Fatalf("unexpected Validate error: %s\n", err)
	}
}
