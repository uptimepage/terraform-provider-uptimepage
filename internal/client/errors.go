package client

import (
	"errors"
	"fmt"
)

// APIError is the decoded form of the API's error envelope:
//
//	{"error":{"code":"...","message":"...","field":"...","trace_id":"..."}}
//
// Code is the stable machine contract — branch on it, never on Message.
type APIError struct {
	Status  int    // HTTP status code
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field"`
	TraceID string `json:"trace_id"`
}

// errorEnvelope matches the wire shape so we can decode the nested object.
type errorEnvelope struct {
	Error APIError `json:"error"`
}

func (e *APIError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (field %q, http %d)", e.Code, e.Message, e.Field, e.Status)
	}
	return fmt.Sprintf("%s: %s (http %d)", e.Code, e.Message, e.Status)
}

// IsNotFound reports whether the error is a missing-resource API error, so a
// resource Read can drop the resource from state instead of failing. Uses
// errors.As so it sees through %w wrapping (e.g. ListTargets wraps its error).
func IsNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == 404
}
