package book

import (
	"fmt"
	"strings"
	"unicode"
)

// ValidationError describes a single field validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validate checks a Book's metadata and returns any validation errors.
func Validate(b *Book) []ValidationError {
	return ValidateMeta(b.Meta)
}

// ValidateMeta checks a Frontmatter directly.
func ValidateMeta(m Frontmatter) []ValidationError {
	var errs []ValidationError

	if strings.TrimSpace(m.Title) == "" {
		errs = append(errs, ValidationError{Field: "title", Message: "must not be empty"})
	}
	if strings.TrimSpace(m.Author) == "" {
		errs = append(errs, ValidationError{Field: "author", Message: "must not be empty"})
	}
	if !IsValidStatus(m.Status) {
		errs = append(errs, ValidationError{
			Field:   "status",
			Message: fmt.Sprintf("unknown status %q", m.Status),
		})
	}
	if m.Rating != nil {
		r := *m.Rating
		if r < 1 || r > 5 {
			errs = append(errs, ValidationError{Field: "rating", Message: "must be between 1 and 5"})
		}
	}
	if m.Started != nil && m.Finished != nil {
		if m.Finished.Before(*m.Started) {
			errs = append(errs, ValidationError{
				Field:   "finished",
				Message: "must not be before started",
			})
		}
	}
	if m.ISBN != "" {
		if err := validateISBN(m.ISBN); err != nil {
			errs = append(errs, ValidationError{Field: "isbn", Message: err.Error()})
		}
	}
	if m.Pages < 0 {
		errs = append(errs, ValidationError{Field: "pages", Message: "must not be negative"})
	}
	if m.Copies < 0 {
		errs = append(errs, ValidationError{Field: "copies", Message: "must not be negative"})
	}

	return errs
}

func validateISBN(isbn string) error {
	// Strip hyphens and spaces before checking length.
	var digits strings.Builder
	for _, ch := range isbn {
		if unicode.IsDigit(ch) {
			digits.WriteRune(ch)
		} else if ch == '-' || ch == ' ' {
			continue
		} else if ch == 'X' || ch == 'x' {
			digits.WriteRune(ch)
		} else {
			return fmt.Errorf("invalid character %q", ch)
		}
	}
	n := digits.Len()
	if n != 10 && n != 13 {
		return fmt.Errorf("must be 10 or 13 digits (got %d)", n)
	}
	return nil
}
