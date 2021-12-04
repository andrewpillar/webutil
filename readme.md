# webutil

webutil is a collection of utility functions to aid in the development of web
applications in Go. this builds on top of some packages provided by the
[Gorilla web toolkit][0] such as [gorilla/schema][1] and [gorilla/sessions][2].

This package provides the ability to easily handle form validation, file
uploads, serving different content types, and flashing of form data between
requests.

## Examples

**Form Validation**

Form validations is achieved via the `webutil.Form` and `webutil.Validator`
interfaces. The `webutil.Form` interface wraps the `Fields` method that returns
a map of the underlying fields in the form. The `webutil.Validator` interface
wraps the `Validate` method for validating data. Below is an example of these
interfaces being implemented for form validation,

    type LoginForm struct {
        Email    string
        Password string
    }

    func (f LoginForm) Fields() map[string]string {
        return map[string]string{
            "email": f.Email,
        }
    }

    type LoginValidator struct {
        Form Login
    }

    func (v LoginValidator) Validate(errs webutil.ValidationErrors) error {
        if f.Email == "" {
            errs.Add("email", webutil.ErrFieldRequired("email"))
        }
        if f.Password == "" {
            errs.Add("password", webutil.ErrFieldRequired("password"))
        }
    }

with the above implementation we can then use `webutil.UnmarshalForm` and
`webutil.Validate` to unmarshal and validate the form data,

    func Login(w http.ResponseWriter, r *http.Request) {
        var f LoginForm

        if err := webutil.UnmarshalForm(&f, r); err != nil {
            io.WriteString(w, err.Error())
            return
        }

        v := LoginValidator{
            Form: f,
        }

        if err := webutil.Validate(v); err != nil {
            io.WriteString(w, err.Error())
            return
        }
    }

`webutil.Validate` will always return the `webutil.ValidationErrors` error
type. Under the hood the [gorilla/schema][1] package is used to handle the
unmarshalling of request data into a form.

**File Uploads**

File uploads can be handled via the `webutil.File` type. This can be used along
the `webutil.FileValidator` to handle the uploading and validating of files,

    type UploadForm struct {
        File *webutil.File
        Name string
    }

    func Upload(w http.ResponseWriter, r *http.Request) {
        f := UploadForm{
            File: &webutil.File{
                Field: "avatar",
            },
        }

        if err := webutil.UnmarshalFormWithFile(&f, f.File, r); err != nil {
            io.WriteString(w, err.Error())
            return
        }

        defer f.File.Remove()

        v := &webutil.FileValidator{
            File: f.File,
            Size: 5 * (1 << 20),
        }

        if err := webutil.Validate(v); err != nil {
            io.WriteString(w, err.Error())
            return
        }

        dir, _ := os.Getwd()
        dst, _ := os.CreateTemp(dir, "")

        io.Copy(dst, f.File)

        w.WriteHeader(http.StatusNoContent)
    }

with the above example, we call the `webutil.UnmarshalFormWithFile` function to
handle the unmarshalling of the file from the request. This will also handle
requests where the file is sent as the request body itself, when this is done
the URL query parameters are used as the typical form values. Validation of the
file is then handled with the `webutil.FileValidator`.

**Response Types**

HTML, Text, and JSON response types can be sent using the respective functions
provided by this package. These functions will set the appropriate
`Content-Type` header, and `Content-Length` too.

    func HTMLHandler(w http.ResponseWriter, r *http.Request) {
        webutil.HTML(w, "<h1>HTML response</h1>", http.StatusOK)
    }

    func TextHandler(w http.ResponseWriter, r *http.Request) {
        webutil.Text(w, "Text response", http.StatusOK)
    }

    func JSONHandler(w http.ResponseWriter, r *http.Request) {
        data := map[string]string{
            "message": "JSON response",
        }
        webutil.JSON(w, data, http.StatusOK)
    }

[0]: https://www.gorillatoolkit.org
[1]: https://github.com/gorilla/schema
[2]: https://github.com/gorilla/sessions
