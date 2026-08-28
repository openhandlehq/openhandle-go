package openhandle

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

type referenceConformanceFixture struct {
	Version int                        `json:"version"`
	Cases   []referenceConformanceCase `json:"cases"`
}

type referenceConformanceCase struct {
	Name       string                    `json:"name"`
	Platform   string                    `json:"platform"`
	Resource   string                    `json:"resource"`
	Input      referenceConformanceInput `json:"input"`
	Identifier string                    `json:"identifier"`
	Error      string                    `json:"error"`
}

type referenceConformanceInput struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

func TestReferenceConformance(t *testing.T) {
	t.Parallel()

	encoded, err := os.ReadFile("testdata/reference-conformance.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture referenceConformanceFixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 1 {
		t.Fatalf("fixture version = %d, want 1", fixture.Version)
	}

	for _, test := range fixture.Cases {
		t.Run(test.Name, func(t *testing.T) {
			identifier, err := resolveConformanceReference(test)
			if test.Error != "" {
				if got := referenceErrorName(err); got != test.Error {
					t.Fatalf("error = %T %v (%q), want %q", err, err, got, test.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if identifier != test.Identifier {
				t.Fatalf("identifier = %q, want %q", identifier, test.Identifier)
			}
		})
	}
}

func resolveConformanceReference(test referenceConformanceCase) (string, error) {
	switch test.Input.Kind {
	case "raw":
		return resolveReference(test.Input.Value, test.Platform, test.Resource)
	case "username":
		return resolveReference(Username(test.Input.Value), test.Platform, test.Resource)
	case "id":
		return resolveReference(ID(test.Input.Value), test.Platform, test.Resource)
	case "url":
		return resolveReference(URL(test.Input.Value), test.Platform, test.Resource)
	default:
		return "", &ReferenceError{Message: "unknown conformance reference kind"}
	}
}

func referenceErrorName(err error) string {
	if err == nil {
		return ""
	}
	var mismatch *ReferenceMismatchError
	if errors.As(err, &mismatch) {
		return "reference_mismatch"
	}
	var referenceError *ReferenceError
	if errors.As(err, &referenceError) {
		return "invalid_reference"
	}
	return "unexpected_error"
}
