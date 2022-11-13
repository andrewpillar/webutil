package webutil

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"hash"
	"image"
	"image/jpeg"
	"io"
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
				URL: &url.URL{
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
					"X-Forwarded-Host": []string{"example.com"},
				},
				URL: &url.URL{
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
				URL: &url.URL{
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
	Body  string `json:"body" schema:"body"`
}

var _ Form = (*Post)(nil)

func (p Post) Fields() map[string]string {
	return map[string]string{
		"title": p.Title,
		"body":  p.Body,
	}
}

func (p Post) Validate(ctx context.Context) error {
	errs := make(ValidationErrors)

	if p.Title == "" {
		errs.Add("title", &FieldError{
			Name: "Title",
			Err:  ErrFieldRequired,
		})
	}
	if p.Body == "" {
		errs.Add("body", &FieldError{
			Name: "Post",
			Err:  ErrFieldRequired,
		})
	}
	return errs.Err()
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
		if err := UnmarshalFormAndValidate(&test.form, test.req); err != nil {
			if test.shoulderr {
				errs, ok := err.(ValidationErrors)

				if !ok {
					t.Errorf("tests[%d] - unexpected error, expected=%T, got=%T(%s)\n", i, ValidationErrors{}, err, err)
				}

				for _, field := range test.errs {
					if _, ok := errs[field]; !ok {
						t.Errorf("tests[%d] - expected field %q in errors\n", i, field)
					}
				}
				continue
			}
			t.Errorf("tests[%d] - unexpected UnmarshalFormAndValidate error: %s\n", i, err)
		}
	}
}

func genimage(t *testing.T) *bytes.Buffer {
	var buf bytes.Buffer

	img := image.NewAlpha(image.Rect(0, 0, 250, 250))

	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 1}); err != nil {
		t.Fatalf("unexpected jpeg.Encode error: %s\n", err)
	}
	return &buf
}

func multipartRequest(t *testing.T, field string, n int) *http.Request {
	pr, pw := io.Pipe()

	mpw := multipart.NewWriter(pw)

	go func() {
		defer mpw.Close()

		w, err := mpw.CreateFormFile(field, "image.jpeg")

		if err != nil {
			t.Fatalf("unexpected multipart.Writer.CreateFormFile error: %s\n", err)
		}
		io.Copy(w, genimage(t))
	}()

	r := httptest.NewRequest("POST", "/upload", pr)
	r.Header.Add("Content-Type", mpw.FormDataContentType())

	return r
}

func Test_UnmarshalFile(t *testing.T) {
	r := multipartRequest(t, "file", 1)

	f, ok, err := UnmarshalFile("file", r)

	if err != nil {
		t.Fatalf("unexpected UnmarshalFile error: %s\n", err)
	}

	if !ok {
		t.Fatal("expected file in request")
	}

	f.Close()
}

func md5sum(r io.Reader) (io.Reader, hash.Hash) {
	md5 := md5.New()
	return io.TeeReader(r, md5), md5
}

func Test_Large_UnmarshalFile(t *testing.T) {
	buf := make([]byte, defaultMaxMemory+10)
	io.ReadFull(rand.Reader, buf)

	tee, expected := md5sum(bytes.NewBuffer(buf))

	req := httptest.NewRequest("POST", "/upload", tee)
	req.Header.Add("Content-Type", "application/octet-stream")

	file, ok, err := UnmarshalFile("", req)

	if err != nil {
		t.Fatalf("unexpected UnmarshalFormWithFile error: %s\n", err)
	}

	if !ok {
		t.Fatalf("expected file to be unmarshalled from request\n")
	}

	r, actual := md5sum(file)

	io.Copy(io.Discard, r)

	if !bytes.Equal(expected.Sum(nil), actual.Sum(nil)) {
		t.Fatalf("unexpected file hash, expected=%q, got=%q\n", expected.Sum(nil), actual.Sum(nil))
	}
}

func Test_UnmarshalFormWithFile(t *testing.T) {
	var (
		post Post
		buf  bytes.Buffer
	)

	mpw := multipart.NewWriter(&buf)

	w, err := mpw.CreateFormFile("attachments", "image.jpeg")

	if err != nil {
		t.Fatalf("unexpected multipart.Writer.CreateFormFile error: %s\n", err)
	}

	io.Copy(w, genimage(t))

	if err := mpw.WriteField("title", "Post with a file"); err != nil {
		t.Fatalf("unexpected multipart.Writer.WriteField error: %s\n", err)
	}
	if err := mpw.WriteField("body", "Post with a file"); err != nil {
		t.Fatalf("unexpected multipart.Writer.WriteField error: %s\n", err)
	}

	mpw.Close()

	r := httptest.NewRequest("POST", "/upload", &buf)
	r.Header.Add("Content-Type", mpw.FormDataContentType())

	_, ok, err := UnmarshalFormWithFile(&post, "attachments", r)

	if err != nil {
		t.Fatalf("unexpected UnmarshalFormWithFile error: %s\n", err)
	}

	if !ok {
		t.Fatalf("expected file to be unmarshalled from request\n")
	}

	if err := post.Validate(r.Context()); err != nil {
		t.Fatalf("unexpected post.Validate error: %s\n", err)
	}
}

func Test_FileBody_UnmarshalFormWithFile(t *testing.T) {
	url, _ := url.Parse("/upload?title=some post&body=some body")

	body := genimage(t)

	r := httptest.NewRequest("POST", "/upload", body)
	r.URL = url
	r.Header.Add("Content-Type", "image/jpeg")

	var post Post

	_, ok, err := UnmarshalFormWithFile(&post, "", r)

	if err != nil {
		t.Fatalf("unexpected UnmarshalFormWithFile error: %s\n", err)
	}

	if !ok {
		t.Fatalf("expected file to be unmarshalled from request\n")
	}

	if err := post.Validate(r.Context()); err != nil {
		t.Fatalf("unexpected post.Validate error: %s\n", err)
	}
}

func Test_UnmarshalFormWithFiles(t *testing.T) {
	var (
		post Post
		buf  bytes.Buffer
	)

	mpw := multipart.NewWriter(&buf)

	for i := 0; i < 5; i++ {
		w, err := mpw.CreateFormFile("attachments", fmt.Sprintf("image%d.jpeg", i))

		if err != nil {
			t.Fatalf("unexpected multipart.Writer.CreateFormFile error: %s\n", err)
		}
		io.Copy(w, genimage(t))
	}

	if err := mpw.WriteField("title", "Post with a file"); err != nil {
		t.Fatalf("unexpected multipart.Writer.WriteField error: %s\n", err)
	}
	if err := mpw.WriteField("body", "Post with a file"); err != nil {
		t.Fatalf("unexpected multipart.Writer.WriteField error: %s\n", err)
	}

	mpw.Close()

	r := httptest.NewRequest("POST", "/upload", &buf)
	r.Header.Add("Content-Type", mpw.FormDataContentType())

	files, ok, err := UnmarshalFormWithFiles(&post, "attachments", r)

	if err != nil {
		t.Fatalf("unexpected UnmarshalFormWithFiles error: %s\n", err)
	}

	if !ok {
		t.Fatalf("expected file to be unmarshalled from request\n")
	}

	if len(files) != 5 {
		t.Fatalf("unexpected files in requests, expected=%d, got=%d\n", 5, len(files))
	}

	for i, f := range files {
		if f.Type != "image/jpeg" {
			t.Fatalf("files[%d] - unexpected file.Type, expected=%q, got=%q\n", i, "image/jpeg", f.Type)
		}
	}
}
