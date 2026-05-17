package member

import (
	"unicode/utf8"

	derrors "github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/shared/errors"
)

const (
	memberNameMin = 1
	memberNameMax = 100
)

// MemberName is a 1..100-character (UTF-8 rune count) display name.
type MemberName struct {
	value string
}

func NewMemberName(s string) (MemberName, error) {
	n := utf8.RuneCountInString(s)
	if n < memberNameMin || n > memberNameMax {
		return MemberName{}, derrors.NewValidationError("name", "must be 1 to 100 characters")
	}
	return MemberName{value: s}, nil
}

func (n MemberName) String() string { return n.value }
