package errs

import (
	"fmt"
	"runtime"

	errs "errors"

	"github.com/pkg/errors"
)

// UserName is a string representing a user
type UserName string

// Kind defines the kind of error this is
type Kind uint8

// Code is a human-readable, short representation of the error
type Code string

// Parameter represents the parameter related to the error.
type Parameter string

// Realm is a description of a protected area, used in the WWW-Authenticate header.
// Realm should be set when error Kind is Unauthenticated. If left unset, Realm
// will be set to the default set by the "restricted" method
type Realm string

// Error is the type that implements the error interface.
// It contains a number of fields, each of different type.
// An Error value may leave some values unset.
type Error struct {
	// User is the username of the user attempting the operation.
	User UserName

	// Kind is the class of error, such as permission failure,
	// or "Other" if its class is unknown or irrelevant.
	Kind Kind

	// Code is a human-readable, short representation of the error
	Code Code

	// Param represents the parameter related to the error.
	Param Parameter

	// Realm is a description of a protected area, used in the WWW-Authenticate header.
	Realm Realm

	// The underlying error that triggered this one, if any.
	Err error
}

// Is is method to satisfy errors.Is interface
func (e *Error) Is(target error) bool {
	return errs.Is(e.Err, target)
}

// As is method to satisfy errors.As interface
func (w *Error) As(target interface{}) bool {
	return errs.As(w.Err, target)
}

func (e *Error) Cause() error {
	return e.Err
}

func (e Error) Unwrap() error {
	return errs.Unwrap(e.Err)
}

func (e *Error) Error() string {
	return e.Err.Error()
}

func (e *Error) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			if st, ok := e.Err.(interface {
				StackTrace() errors.StackTrace
			}); ok {
				for _, fr := range st.StackTrace()[1:] {
					fmt.Fprintf(s, "\n%+v", fr)
				}
				return
			}
		}
		fallthrough
	case 's':
		fmt.Fprintf(s, "%s", e.Error())
	}
}

func (e *Error) isZero() bool {
	return e.Is(ErrUndefined) &&
		e.User == "" &&
		e.Param == "" &&
		e.Code == ""
}

const (
	Other          Kind = iota // Unclassified error. This value is not printed in the error message.
	IO                         // External I/O error such as network failure
	Private                    // Information withheld
	Internal                   // Internal error or inconsistency
	Database                   // Database error
	Exist                      // Resource already exist
	NotExist                   // Resource does not exists
	Invalid                    // Invalid operation for this type of item
	Validation                 // Input validation error
	InvalidRequest             // Invalid request

	// Unauthenticated error will response with http.StatusUnauthorized (401) with empty body
	Unauthenticated

	// Unauthorized error will response with http.StatusForbidden (403) with empty body
	// It is used when an authenticated user trying to access the resource
	// but not permitted to do so
	Unauthorized
)

func (k Kind) String() string {
	switch k {
	case Other:
		return "other_error"
	case IO:
		return "I/O_error"
	case Private:
		return "private"
	case Internal:
		return "internal_error"
	case Database:
		return "database_error"
	case Exist:
		return "resource_already_exists"
	case NotExist:
		return "resource_does_not_exist"
	case Invalid:
		return "invalid_operation"
	case Validation:
		return "input_validation_error"
	case InvalidRequest:
		return "invalid_request_error"
	case Unauthenticated:
		return "unauthenticated_request"
	case Unauthorized:
		return "unauthorized_request"
	}

	return "unknown_error"
}

var DefaultRealm Realm = "restricted"
var ErrUndefined = errors.New("undefined error")

// E builds an error value from its arguments.
// There must be at least one argument or E panics.
// The type of each argument determines its meaning.
// If more than one argument of a given type is presented,
// only the last one is recorded.
//
// The types are:
//	errs.UserName
//		The username of the user attempting the operation.
//	errs.Kind
//		The class of error, such as permission failure.
//	errs.Code
//		The code for a human-readable purpose about the error.
//	errs.Parameter
//		The parameter represent the parameter related with the error.
//	string
//		Treated as an error message and assigned to the
//		Err field after a call to errors.New.
//	error
//		The underlying error that triggered this one, if the error not contains stack,
// 		we will wrap it
//
// If the error is printed, only those items that have been
// set to non-zero values will appear in the result.
//
// If Kind is not specified or Other, we set it to the Kind of
// the underlying error.
func E(args ...interface{}) error {

	if len(args) == 0 {
		panic("call to errs.E with no arguments")
	}

	e := &Error{}
	for _, arg := range args {
		switch arg := arg.(type) {
		case Kind:
			e.Kind = arg
		case UserName:
			e.User = arg
		case Code:
			e.Code = arg
		case Parameter:
			e.Param = arg
		case Realm:
			e.Realm = arg
		case string:
			e.Err = errors.New(arg)
		case *Error:
			e.Err = arg
		case error:
			// if the error is validation errors, skipping the stacktrace
			if verr, ok := arg.(ValidationErrors); ok {
				e.Err = verr
				continue
			}

			// if the error implements stackTracer, then it is
			// a pkg/errors error type and does not need to have
			// the stack added
			_, ok := arg.(interface {
				StackTrace() errors.StackTrace
			})
			if ok {
				e.Err = arg
			} else {
				e.Err = errors.WithStack(arg)
			}

		default:
			_, file, line, _ := runtime.Caller(1)
			return fmt.Errorf("errs.E: bad call from %s:%d: %v, unknown type %T, value %v in error call", file, line, args, arg, arg)
		}
	}

	// If this error and the inner still has Realm == "", while error Kind is Unauthenticated
	// then the realm set to default "restricted" method
	if e.Realm == "" && e.Kind == Unauthenticated {
		e.Realm = DefaultRealm
	}

	if e.Err == nil {
		e.Err = ErrUndefined
	}

	prev, ok := e.Err.(*Error)
	if !ok {
		return e
	}
	// If this error has Kind unset or Other, pull up the inner one.
	if e.Kind == Other {
		e.Kind = prev.Kind
		prev.Kind = Other
	}

	if prev.Code == e.Code {
		prev.Code = ""
	}
	// If this error has Code == "", pull up the inner one.
	if e.Code == "" {
		e.Code = prev.Code
		prev.Code = ""
	}

	if prev.Param == e.Param {
		prev.Param = ""
	}
	// If this error has Param == "", pull up the inner one.
	if e.Param == "" {
		e.Param = prev.Param
		prev.Param = ""
	}

	if prev.Realm == e.Realm {
		prev.Realm = ""
	}

	// If this inner error has Realm, pull up the inner one
	if e.Realm == "" {
		e.Realm = prev.Realm
		prev.Realm = ""
	}

	return e
}

// Match compares its two error arguments. It can be used to check
// for expected errors in tests. Both arguments must have underlying
// type *Error or Match will return false. Otherwise it returns true
// if every non-zero element of the first error is equal to the
// corresponding element of the second.
// If the Err field is a *Error, Match recurs on that field;
// otherwise it compares the strings returned by the Error methods.
// Elements that are in the second argument but not present in
// the first are ignored.
//
// For example,
//	Match(errs.E(errors.Permission, errs.UserName("john@doe.com")), err)
//  tests whether err is an Error with Kind=Permission and User=john@doe.com.
func Match(err1, err2 error) bool {
	e1, ok := err1.(*Error)
	if !ok {
		return false
	}
	e2, ok := err2.(*Error)
	if !ok {
		return false
	}
	if e1.User != "" && e2.User != e1.User {
		return false
	}
	if e1.Kind != Other && e2.Kind != e1.Kind {
		return false
	}
	if e1.Param != "" && e2.Param != e1.Param {
		return false
	}
	if e1.Code != "" && e2.Code != e1.Code {
		return false
	}
	if e1.Err != nil {
		if _, ok := e1.Err.(*Error); ok {
			return Match(e1.Err, e2.Err)
		}
		if e2.Err == nil || e2.Err.Error() != e1.Err.Error() {
			return false
		}
	}
	return true
}

// KindIs reports whether err is an *Error of the given Kind.
// If err is nil then KindIs returns false.
func KindIs(kind Kind, err error) bool {
	e, ok := err.(*Error)
	if !ok {
		return false
	}

	return e.Kind == kind
}
