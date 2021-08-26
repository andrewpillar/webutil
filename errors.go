package webutil

import (
	"errors"
	"fmt"
	"strings"
)

// Errors records the errors that occur during form validation. Each key is a
// field within the form that erred, and the values are the list of error
// messages.
type Errors map[string][]string

// UnmarshalError records the error that occurred during the unmarshalling of
// a field in a form.
type UnmarshalError struct {
	Field string
	Err   error
}

// FieldError records an error against the given field.
type FieldError struct {
	Field string
	Err   error
}

var (
	ErrRequired = errors.New("field is required")
	ErrExists   = errors.New("already exists")
)

// ErrField records the given error for the given field.
func ErrField(field string, err error) error {
	return &FieldError{
		Field: field,
		Err:   err,
	}
}

// ErrFieldExists records an error should the given field's value already
// exist, for example an email in a database.
func ErrFieldExists(field string) error {
	return &FieldError{
		Field: field,
		Err:   ErrExists,
	}
}

// ErrFieldRequired records an error for a field that was not provided in a
// form.
func ErrFieldRequired(field string) error {
	return &FieldError{
		Field: field,
		Err:   ErrRequired,
	}
}

// NewErrors returns an empty set of Errors.
func NewErrors() *Errors {
	errs := Errors(make(map[string][]string))
	return &errs
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("%s %s", e.Field, e.Err.Error())
}

func (e *FieldError) Unwrap() error {
	return e.Err
}

// Err returns the underlying error for the current set of Errors. If there are
// no errors recorded, then this returns nil.
func (e *Errors) Err() error {
	if len((*e)) == 0 {
		return nil
	}
	return e
}

// Has checks to see if the given error exists in the set for the given field.
func (e *Errors) Has(field string, err error) bool {
	errs, ok := (*e)[field]

	if ok {
		return false
	}

	for _, err2 := range errs {
		if err.Error() == err2 {
			return true
		}
	}
	return false
}

// Error builds a formatted string of the errors in the set, the final string
// is formatted like so,
//
//     field:
//         error
func (e *Errors) Error() string {
	var buf strings.Builder

	for field, errs := range (*e) {
		buf.WriteString(field + ":\n")

		for _, err := range errs {
			buf.WriteString("    " + err + "\n")
		}
	}
	return buf.String()
}

// First returns the first error message that can be found for the given field.
// If no message can be found then an empty string is returned.
func (e *Errors) First(key string) string {
	errs, ok := (*e)[key]

	if !ok {
		return ""
	}

	if len(errs) == 0 {
		return ""
	}
	return errs[0]
}

// Put appends the given error message to the given key in the set.
func (e *Errors) Put(key string, err error) {
	if err == nil {
		return
	}
	(*e)[key] = append((*e)[key], err.Error())
}

// Merge merges the set of errors from e1 into the  given set.
func (e *Errors) Merge(e1 *Errors) {
	for field, errs := range (*e1) {
		(*e)[field] = append((*e)[field], errs...)
	}
}

// Error returns the formatted string of the UnmarshalError.
func (e UnmarshalError) Error() string {
	return "failed to unmarshal " + e.Field + ": " + e.Err.Error()
}
