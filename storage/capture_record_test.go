package storage

import (
	"regexp"
	"testing"
)

func TestUUIDString(t *testing.T) {
	u := UUID{
		0xf4, 0x7a, 0xc1, 0x0b, 0x58, 0xcc, 0x43, 0x72,
		0xa5, 0x67, 0x0e, 0x02, 0xb2, 0xc3, 0xd4, 0x79,
	}
	got := u.String()
	want := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	if got != want {
		t.Fatalf("UUID.String() = %q, want %q", got, want)
	}
}

func TestUUIDStringZero(t *testing.T) {
	var u UUID // all zeros — the fallback when generation fails
	want := "00000000-0000-0000-0000-000000000000"
	if got := u.String(); got != want {
		t.Fatalf("zero UUID.String() = %q, want %q", got, want)
	}
}

func TestNewUUIDStringIsCanonicalV4(t *testing.T) {
	canonical := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for i := 0; i < 100; i++ {
		u, err := NewUUID()
		if err != nil {
			t.Fatalf("NewUUID: %v", err)
		}
		if s := u.String(); !canonical.MatchString(s) {
			t.Fatalf("NewUUID().String() = %q is not a canonical v4 UUID", s)
		}
	}
}
