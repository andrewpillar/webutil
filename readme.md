# webutil

webutil is a collection of utility functions to aid in the development of web
applications in Go. this builds on top of some packages provided by the
[Gorilla web toolkit](https://www.gorillatoolkit.org/) such as
[gorilla/schema](https://github.com/gorilla/schema) and
[gorilla/sessions](https://github.com/gorilla/sessions).

This package provides the ability to easily handle form validation, file
uploads, serving different content types, and flashing of form data between
requests.

## Examples

**Form Validation**

Form validation is achieved via the `webutil.Form` method that wraps the
`Fields`, and `Validate` methods. The `Validate` method is what is called
to actually perform the form validation, and the `Fields` method is what's
called when form data is flashed to the session. Below is an example of a
form implementation,

    type Login struct {
        Email    string `schema:"email"`
        Password string `schema:"password"`
    }

    func (f Login) Fields() map[string]string {
        return map[string]string{
            "email": f.Email,
        }
    }

    func (f Login) Validate() error {
        errs := webutil.NewErrors()

        if f.Email == "" {
            errs.Put("email", webutil.ErrFieldRequired("email"))
        }

        if f.Password == "" {
            errs.Put("password", webutil.ErrFieldRequired("password"))
        }
        return errs.Err()
    }

the [gorilla/schema](https://github.com/gorilla/schema) package is used to
handle the unmarshalling of request data into a form.

Each implementation of the `webutil.Form` interface should return the
`*webutil.Errors` type containg any validation errors that occur. If any
other errors occur during the invocation of `Validate`, (such as a database
connection error), then it is fine to return these directly.

**File Uploads**

File uploads can be handled via the `webutil.File` type that can be created
via `webutil.NewFile`. Below is an example of handling file uploads, elided for
brevity,

    var store = sessions.NewCookieStore([]byte(os.Getenv("SESSION_KEY")))

    func Upload(w http.ResponseWriter, r *http.Request) {
       sess, _ := store.Get(r, "session")

        f := webutil.NewFile("avatar", 5 * (1 << 20), r)
        f.Allow("image/png", "image/jpeg")

        if err := webutil.UnmarshalAndValidate(f, r); err != nil {
            errs, ok := err.(*webutil.Errors)

            if ok {
                webutil.FlashFormWithErrors(sess, f, errs)
                sess.Save(r, w)
                http.Redirect(w, r, r.Header.Get("Referer"), http.StatusSeeOther)
                return
            }
            panic(errs) // don't actually do this
        }

        dir, _ := os.Getwd()
        dst, _ := ioutil.TempFile(dir, "")

        // Store the file on disk.
        io.Copy(dst, f)

        w.WriteHeader(http.StatusOK)
    }

we specify that a file upload is going to take place via the `NewFile` function,
this will return `*webutil.File` for handling the upload and validation of
files. The `Allow` method is then called to tell it that we only want to allow
files with the given MIME types. Finally we then pass this to
`UnmarshalAndValidate`. This is the function that actually parses the request
data and validates it. If any validation errors do occur, then
`*webutil.Errors` will be returned. We then flash this information to the
session, and redirect back.

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
