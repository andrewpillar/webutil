package webutil

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ValidatorFunc is the function type for validating a value in a form. This
// will return an error should validation fail.
type ValidatorFunc func(ctx context.Context, val any) error

type fieldValidator struct {
	name     string
	val      any
	validate ValidatorFunc
}

// WrapErrorFunc is the function type for wrapping an error that is returned
// from a failed validation. This would be given the name of the field and the
// error for that failed validation. This would be used for either adding
// additional context to an error, or mutating it.
type WrapErrorFunc func(name string, err error) error

// Validator is the type used for validating data.
type Validator struct {
	fields []*fieldValidator
	wraps  []WrapErrorFunc
}

// WrapFieldError wraps the given error with the *FieldError type and returns
// it.
func WrapFieldError(name string, err error) error {
	return &FieldError{
		Name: strings.Title(name),
		Err:  err,
	}
}

// MapError will map the given error from to the given error of to if the
// underlying error is of the type from. The underlying check is done via
// errors.Is.
func MapError(from, to error) WrapErrorFunc {
	return func(name string, err error) error {
		if errors.Is(err, from) {
			return to
		}
		return err
	}
}

// IgnoreError will return nil if the underlying validation error and field
// name match what is given as name and target. This is useful if there are
// benign errors you want to ignore during validation.
func IgnoreError(name string, target error) WrapErrorFunc {
	return func(name2 string, err error) error {
		if name == name2 {
			if errors.Is(err, target) {
				return nil
			}
		}
		return err
	}
}

// WrapError sets the chain of WrapErrorFuncs to use when processing a
// validation error.
func (v *Validator) WrapError(wraps ...WrapErrorFunc) { v.wraps = wraps }

// Add will add a ValidatorFunc to the given Validator for the field of name and
// with the value of val.
func (v *Validator) Add(name string, val any, fn ValidatorFunc) {
	if v.fields == nil {
		v.fields = make([]*fieldValidator, 0)
	}

	v.fields = append(v.fields, &fieldValidator{
		name:     name,
		val:      val,
		validate: fn,
	})
}

// Validate runs the validatio functions and returns all errors via
// ValidationErrors. When calling this, a subsequent call to Err should be made
// on the returned ValidationErrors, this will either return an error or nil
// depending on whether or not ValidationErrors contains errors.
func (v *Validator) Validate(ctx context.Context) ValidationErrors {
	errs := make(ValidationErrors)

	if len(v.wraps) == 0 {
		v.wraps = []WrapErrorFunc{
			WrapFieldError,
		}
	}

	for _, fld := range v.fields {
		if err := fld.validate(ctx, fld.val); err != nil {
			for _, wrap := range v.wraps {
				err = wrap(fld.name, err)

				if err == nil {
					break
				}
			}
			errs.Add(fld.name, err)
		}
	}
	return errs
}

// FieldRequired checks to see if the given val was actually given. This
// only checks if val is a string, or has the String method on it, otherwise
// nil is returned.
func FieldRequired(ctx context.Context, val any) error {
	switch v := val.(type) {
	case interface { String() string }:
		if v.String() == "" {
			return ErrFieldRequired
		}
	case string:
		if v == "" {
			return ErrFieldRequired
		}
	}
	return nil
}

type MatchError struct {
	Regexp *regexp.Regexp
}

func (e MatchError) Error() string {
	return "does not match " + e.Regexp.String()
}

// FieldMatches checks to see if the given val matches the regular expression
// of re. This will return an error of type MatchError if validation fails.
func FieldMatches(re *regexp.Regexp) ValidatorFunc {
	return func(ctx context.Context, val any) error {
		s, _ := val.(string)

		if !re.Match([]byte(s)) {
			return MatchError{
				Regexp: re,
			}
		}
		return nil
	}
}

// FieldLen checks to see if the given val is between the length of min and max.
func FieldLen(min, max int) ValidatorFunc {
	return func(ctx context.Context, val any) error {
		s, _ := val.(string)
		l := len(s)

		if l < min || l > max {
			return fmt.Errorf("must be between %d and %d characters in length", min, max)
		}
		return nil
	}
}

// FieldMinLen checks to see if the given val is at least longer than min.
func FieldMinLen(min int) ValidatorFunc {
	return func(ctx context.Context, val any) error {
		s, _ := val.(string)

		if len(s) < min {
			return fmt.Errorf("cannot be shorter than %d characters in length", min)
		}
		return nil
	}
}

// FieldMaxLen checks to see if the given val is at least shorter than max.
func FieldMaxLen(max int) ValidatorFunc {
	return func(ctx context.Context, val any) error {
		s, _ := val.(string)

		if len(s) > max {
			return fmt.Errorf("cannot be longer than %d characters in length", max)
		}
		return nil
	}
}

// FieldEquals checks to see if the given val matches expected.
func FieldEquals(expected any) ValidatorFunc {
	return func(ctx context.Context, val any) error {
		if val != expected {
			return errors.New("does not match")
		}
		return nil
	}
}
