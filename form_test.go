package webutil

import (
	"bytes"
	"image"
	"image/jpeg"
	"io"
	"io/ioutil"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type Post struct {
	Title  string `schema:"title"`
	Body   string `schema:"body"`
}

var _ Form = (*Post)(nil)

func spoofFile(t *testing.T) *http.Request {
	pr, pw := io.Pipe()

	mpw := multipart.NewWriter(pw)

	go func() {
		defer mpw.Close()

		w, err := mpw.CreateFormFile("avatar", "dog-in-hat-and-scarf.jpg")

		if err != nil {
			t.Fatalf("unexpected multipart.Writer.CreateFormFile error: %s\n", err)
		}

		img := image.NewAlpha(
			image.Rect(rand.Intn(255), rand.Intn(255), rand.Intn(255), rand.Intn(255)),
		)

		if err := jpeg.Encode(w, img, &jpeg.Options{Quality: 1}); err != nil {
			t.Fatalf("unexpected jpeg.Encode error: %s\n", err)
		}
	}()

	r := httptest.NewRequest("POST", "/upload", pr)
	r.Header.Add("Content-Type", mpw.FormDataContentType())

	return r
}

func Test_FileAllow(t *testing.T) {
	r := spoofFile(t)

	f := NewFile("avatar", 0, r)
	f.Allow("image/jpeg")

	if err := UnmarshalAndValidate(f, r); err != nil {
		t.Fatalf("unexpected UnmarshalAndValidate error: %s\n", err)
	}
	io.Copy(ioutil.Discard, f)
}

func Test_FileDisallow(t *testing.T) {
	r := spoofFile(t)

	f := NewFile("avatar", 0, r)
	f.Disallow("image/jpeg")

	if err := UnmarshalAndValidate(f, r); err == nil {
		t.Fatalf("expected UnmarshalAndValidate to error\n")
	}
}

func Test_FileLimit(t *testing.T) {
	r := spoofFile(t)

	f := NewFile("avatar", 1, r)

	err := UnmarshalAndValidate(f, r)

	if err == nil {
		t.Fatalf("expected UnmarshalAndValidate to error\n")
	}

	ferrs, ok := err.(*Errors)

	if !ok {
		t.Fatalf("unexpected error type, expected=%T, got=%T", NewErrors(), ferrs)
	}

	expected := "avatar cannot be bigger than 1 B"

	if msg := ferrs.First("avatar"); msg != expected {
		t.Fatalf("unexpected error, expected=%q, got=%q\n", expected, msg)
	}
}

func Test_Form(t *testing.T) {
	tests := []struct {
		form        Post
		request     *http.Request
		shouldError bool
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
		},
		{
			Post{},
			&http.Request{
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: ioutil.NopCloser(bytes.NewBufferString(`{"title": "my post", "body": "body"}`)),
			},
			false,
		},
		{
			Post{},
			&http.Request{},
			true,
		},
	}

	for i, test := range tests {
		if err := UnmarshalAndValidate(&test.form, test.request); err != nil {
			if !test.shouldError {
				t.Fatalf("tests[%d] - unexpected UnmarshalAndValidate error: %s\n", i, err)
			}
		}
	}
}

func (f Post) Validate() error {
	errs := NewErrors()

	if f.Title == "" {
		errs.Put("title", ErrFieldRequired("Title"))
	}
	if f.Body == "" {
		errs.Put("body", ErrFieldRequired("Body"))
	}
	return errs.Err()
}

func (f Post) Fields() map[string]string {
	return map[string]string{
		"title": f.Title,
		"body":  f.Body,
	}
}
