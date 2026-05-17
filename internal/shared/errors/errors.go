// Package errors defines the four domain error types used across all BCs.
//
// Handlers translate these to transport-level errors (gRPC status in Step 0+1,
// REST/HTTP in Step N) via errors.As. The mapping table lives in ADR 0009.
package errors

import "fmt"

// ValidationError indicates that input failed structural or semantic validation
// inside a value object or usecase precondition check (e.g., malformed email,
// out-of-range quantity). Maps to gRPC InvalidArgument / HTTP 400.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation: %s: %s", e.Field, e.Message)
	}
	return "validation: " + e.Message
}

func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

// NotFoundError indicates a lookup miss for an aggregate or entity by identity.
// Maps to gRPC NotFound / HTTP 404.
type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("not found: %s id=%s", e.Resource, e.ID)
}

func NewNotFoundError(resource, id string) *NotFoundError {
	return &NotFoundError{Resource: resource, ID: id}
}

// ConflictError indicates a uniqueness or identity conflict (e.g., duplicate
// email registration). Maps to gRPC AlreadyExists / HTTP 409.
type ConflictError struct {
	Resource string
	Message  string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("conflict: %s: %s", e.Resource, e.Message)
}

func NewConflictError(resource, message string) *ConflictError {
	return &ConflictError{Resource: resource, Message: message}
}

// DomainError indicates a business-rule violation that is neither a
// validation issue nor a not-found / conflict (e.g., insufficient stock,
// invalid state transition). Maps to gRPC FailedPrecondition / HTTP 422.
type DomainError struct {
	Code    string
	Message string
}

func (e *DomainError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("domain[%s]: %s", e.Code, e.Message)
	}
	return "domain: " + e.Message
}

func NewDomainError(code, message string) *DomainError {
	return &DomainError{Code: code, Message: message}
}
