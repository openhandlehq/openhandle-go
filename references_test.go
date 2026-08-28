package openhandle

import (
	"errors"
	"testing"
)

func TestResolveProfileReferences(t *testing.T) {
	t.Parallel()

	assertResolved(t, "raw username", "openai", "instagram", "profile", "@openai")
	assertResolved(t, "raw leading at", "@openai", "instagram", "profile", "@openai")
	assertResolved(t, "raw numeric username", "12356", "instagram", "profile", "@12356")
	assertResolved(t, "raw dotted username", "instagram.com", "instagram", "profile", "@instagram.com")
	assertResolved(t, "explicit username", Username("12356"), "instagram", "profile", "@12356")
	assertResolved(t, "opaque id", ID("25025320"), "instagram", "profile", "25025320")
	assertResolved(t, "explicit profile url", URL("instagram.com/openai/"), "instagram", "profile", "@openai")
	assertResolved(t, "raw profile url", "https://www.instagram.com/openai/", "instagram", "profile", "@openai")
	assertResolved(t, "schemeless raw profile url", "instagram.com/openai/", "instagram", "profile", "@openai")
}

func assertResolved[T ProfileReference](t *testing.T, name string, input T, platform, resource, want string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		got, err := resolveReference(input, platform, resource)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("resolveReference() = %q, want %q", got, want)
		}
	})
}

func TestResolveReferenceRejectsMismatch(t *testing.T) {
	t.Parallel()

	_, err := resolveReference(URL("https://www.instagram.com/p/Db04otPRpRH/"), "instagram", "profile")
	var mismatch *ReferenceMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("error = %T %v, want ReferenceMismatchError", err, err)
	}
	if mismatch.ActualResource != "post" || mismatch.ExpectedResource != "profile" {
		t.Fatalf("unexpected mismatch: %#v", mismatch)
	}
}

func TestRawURLRejectsMismatch(t *testing.T) {
	t.Parallel()

	_, err := resolveReference("https://www.instagram.com/p/Db04otPRpRH/", "instagram", "profile")
	var mismatch *ReferenceMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("error = %T %v, want ReferenceMismatchError", err, err)
	}
}

func TestRawURLDoesNotFallBackAfterClassification(t *testing.T) {
	t.Parallel()

	_, err := resolveReference("instagram.com/%zz", "instagram", "profile")
	var referenceError *ReferenceError
	if !errors.As(err, &referenceError) {
		t.Fatalf("error = %T %v, want ReferenceError", err, err)
	}
	if referenceError.Message != "invalid social URL" {
		t.Fatalf("message = %q, want invalid social URL", referenceError.Message)
	}
}

func TestResolveRawResourceReference(t *testing.T) {
	t.Parallel()

	assertResolved(t, "post shorthand", "Db04otPRpRH", "instagram", "post", "Db04otPRpRH")
	assertResolved(t, "post URL", "instagram.com/p/Db04otPRpRH/", "instagram", "post", "Db04otPRpRH")
}

func TestResolveReferenceRejectsUsernameForNonProfile(t *testing.T) {
	t.Parallel()

	_, err := resolveReference(Username("openai"), "instagram", "post")
	var referenceError *ReferenceError
	if !errors.As(err, &referenceError) {
		t.Fatalf("error = %T %v, want ReferenceError", err, err)
	}
}
