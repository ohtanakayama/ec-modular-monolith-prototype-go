package member

import (
	"regexp"

	derrors "github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/shared/errors"
)

// Email is a value object holding a syntactically-valid email address.
// The zero value is invalid; always construct via NewEmail.
type Email struct {
	value string
}

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func NewEmail(s string) (Email, error) {
	if !emailRe.MatchString(s) {
		return Email{}, derrors.NewValidationError("email", "invalid email format")
	}
	return Email{value: s}, nil
}

func (e Email) String() string { return e.value }
