// Package errors contains helper functions and types to work with errors
package errors

import (
	"errors"
	"net/http"
)

// Category defines error category
type Category int

const (
	// CategoryNoError is for the datadog request tracking, in case if internal services (GRPC) returns no error.
	CategoryNoError Category = iota
	// CategoryDataError The client sends some invalid data in the request,
	// for example, missing or incorrect content in the payload or parameters.
	// Could also represent a generic client error.
	CategoryDataError
	// CategoryUnauthorized The client is not authorized to access the requested resource
	CategoryUnauthorized
	// CategoryForbidden The client is not authenticated to access the requested resource
	CategoryForbidden
	// CategoryResourceNotFound The client is attempting to access a resource that does not exist
	CategoryResourceNotFound
	// CategoryNotSupported The requested functionality is not supported
	CategoryNotSupported
	// CategoryDataConflict The client send some data that can create conflict with existing data
	CategoryDataConflict
	// CategoryLocked The client is not able to access the requested resource due to its locked state
	CategoryLocked
	// CategoryDependencyFailure A dependent service is throwing errors
	CategoryDependencyFailure
	// CategoryGeneralError The service failed in an unexpected way
	CategoryGeneralError
	// CategoryRecovering The service is failing but is expected to recover
	CategoryRecovering
	// CategoryConnectionTimeout Connection to a dependent service timing out
	CategoryConnectionTimeout
)

func (c Category) String() string {
	switch c {
	case CategoryDataError:
		return "CategoryDataError"
	case CategoryUnauthorized:
		return "CategoryUnauthorized"
	case CategoryForbidden:
		return "CategoryForbidden"
	case CategoryResourceNotFound:
		return "CategoryResourceNotFound"
	case CategoryNotSupported:
		return "CategoryNotSupported"
	case CategoryDataConflict:
		return "CategoryDataConflict"
	case CategoryLocked:
		return "CategoryLocked"
	case CategoryDependencyFailure:
		return "CategoryDependencyFailure"
	case CategoryRecovering:
		return "CategoryRecovering"
	case CategoryConnectionTimeout:
		return "CategoryConnectionTimeout"
	default:
		return "CategoryGeneralError"
	}
}

// ServiceError represents service specific type that
// is used all over the services.
type ServiceError struct {
	Category Category
	Message  string
	Err      error
}

// Error method to comply with error interface
func (err ServiceError) Error() string {
	if err.Err != nil {
		return err.Err.Error()
	}
	return err.Message
}

// Unwrap returns the underlying error
func (err ServiceError) Unwrap() error {
	return err.Err
}

// Is implements the custom condition to check an error is equal to a service error
func (err ServiceError) Is(target error) bool {
	return err.Message == target.Error()
}

// Is checks that provided error is a ServiceError with desired Category
func Is(err error, cat Category) bool {
	var svcErr *ServiceError
	if errors.As(err, &svcErr) && svcErr.Category == cat {
		return true
	}
	return false
}

// IsInternalError checks that provided error is a Internal system error
func IsInternalError(err error) bool {
	var svcErr *ServiceError
	if errors.As(err, &svcErr) && (svcErr.Category < CategoryDependencyFailure) {
		return false
	}
	return true
}

// GeneralError returns a general service error
// this error mesage sent to the user is "Internal Server Error"
// the error passed is logged in the logger
func GeneralError(err error) error {
	if err == nil {
		err = errors.New("internal server error")
	}
	return &ServiceError{
		Category: CategoryGeneralError,
		Message:  "Internal Server Error",
		Err:      err,
	}
}

// ResourceNotFoundError returns an error with category ResourceNotFound
// the error message provided is returned to the user
// the err object provided is logged in logger
func ResourceNotFoundError(err error, message string) error {
	if err == nil {
		err = errors.New("resource not found:" + message)
	}
	return &ServiceError{
		Category: CategoryResourceNotFound,
		Message:  message,
		Err:      err,
	}
}

// BadRequestError returns  an error with category DataError
// the error message provided is returned to the user
// the error object provided is logged in logger
func BadRequestError(err error, message string) error {
	if err == nil {
		err = errors.New("bad request:" + message)
	}
	return &ServiceError{
		Category: CategoryDataError,
		Message:  message,
		Err:      err,
	}
}

// NotSupportedError returns  an error with category NotSupported
// the error message provided is returned to the user
// the error object provided is logged in logger
func NotSupportedError(err error, message string) error {
	if err == nil {
		err = errors.New("not supported:" + message)
	}
	return &ServiceError{
		Category: CategoryNotSupported,
		Message:  message,
		Err:      err,
	}
}

// ForbiddenError returns a an error with category CategoryForbidden
// the error message provided is returned to the user
// the error object provided is logged in logger
func ForbiddenError(err error, message string) error {
	if err == nil {
		err = errors.New("request forbidden")
	}
	return &ServiceError{
		Category: CategoryForbidden,
		Message:  message,
		Err:      err,
	}
}

// UnAuthorizedError returns an error with category CategoryUnauthorized
// the error message provided is returned to the user
// the error object provided is logged in logger
func UnAuthorizedError(err error, message string) error {
	if err == nil {
		err = errors.New("unauthorized")
	}
	return &ServiceError{
		Category: CategoryUnauthorized,
		Message:  message,
		Err:      err,
	}
}

// ConflictError returns an error with category CategoryDataConflict
// the error message provided is returned to the user
// the error object provided is logged in logger
func ConflictError(err error, message string) error {
	if err == nil {
		err = errors.New("conflict")
	}
	return &ServiceError{
		Category: CategoryDataConflict,
		Message:  message,
		Err:      err,
	}
}

// StatusCode returns the HTTP status code for the error category
func (err ServiceError) StatusCode() int {
	switch err.Category {
	case CategoryDataError:
		return http.StatusBadRequest
	case CategoryUnauthorized:
		return http.StatusUnauthorized
	case CategoryForbidden:
		return http.StatusForbidden
	case CategoryResourceNotFound:
		return http.StatusNotFound
	case CategoryNotSupported:
		return http.StatusMethodNotAllowed
	case CategoryDataConflict:
		return http.StatusConflict
	case CategoryLocked:
		return http.StatusLocked
	case CategoryDependencyFailure:
		return http.StatusBadGateway
	case CategoryGeneralError:
		return http.StatusInternalServerError
	case CategoryRecovering:
		return http.StatusServiceUnavailable
	case CategoryConnectionTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}
