package member_test

import (
	"errors"
	"testing"

	derrors "github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/shared/errors"
	"github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/modules/members/domain/member"
)

func TestNewEmail_Valid(t *testing.T) {
	cases := []string{
		"a@b.co",
		"alice+tag@example.com",
		"user.name@sub.example.co.jp",
	}
	for _, c := range cases {
		e, err := member.NewEmail(c)
		if err != nil {
			t.Fatalf("NewEmail(%q) unexpected error: %v", c, err)
		}
		if e.String() != c {
			t.Fatalf("Email.String() = %q, want %q", e.String(), c)
		}
	}
}

func TestNewEmail_Invalid_ReturnsValidationError(t *testing.T) {
	cases := []string{
		"",
		"plainstring",
		"missing@dotcom",
		"@no-local.com",
		"no-domain@",
		"two parts @example.com",
	}
	for _, c := range cases {
		_, err := member.NewEmail(c)
		if err == nil {
			t.Fatalf("NewEmail(%q) expected error, got nil", c)
		}
		var ve *derrors.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("NewEmail(%q) error is not *ValidationError: %v", c, err)
		}
		if ve.Field != "email" {
			t.Fatalf("ValidationError.Field = %q, want %q", ve.Field, "email")
		}
	}
}
