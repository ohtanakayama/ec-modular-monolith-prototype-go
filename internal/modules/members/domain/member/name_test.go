package member_test

import (
	"errors"
	"strings"
	"testing"

	derrors "github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/shared/errors"
	"github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/modules/members/domain/member"
)

func TestNewMemberName_BoundsAccepted(t *testing.T) {
	cases := []string{
		"A",                           // 1 rune
		strings.Repeat("a", 100),      // 100 ASCII runes
		strings.Repeat("田", 100),     // 100 multibyte runes (rune count, not byte count)
		"山田 太郎",
	}
	for _, c := range cases {
		n, err := member.NewMemberName(c)
		if err != nil {
			t.Fatalf("NewMemberName(%q) unexpected error: %v", c, err)
		}
		if n.String() != c {
			t.Fatalf("MemberName.String() = %q, want %q", n.String(), c)
		}
	}
}

func TestNewMemberName_OutOfRange_ReturnsValidationError(t *testing.T) {
	cases := []string{
		"",                            // 0 runes
		strings.Repeat("a", 101),      // 101 runes
		strings.Repeat("田", 101),     // 101 multibyte runes
	}
	for _, c := range cases {
		_, err := member.NewMemberName(c)
		if err == nil {
			t.Fatalf("NewMemberName(len=%d) expected error, got nil", len(c))
		}
		var ve *derrors.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("NewMemberName(len=%d) error is not *ValidationError: %v", len(c), err)
		}
		if ve.Field != "name" {
			t.Fatalf("ValidationError.Field = %q, want %q", ve.Field, "name")
		}
	}
}
