package book

import (
	"fmt"
	"time"
)

// Status represents the reading lifecycle of a book.
type Status string

const (
	StatusAntilibrary Status = "antilibrary"
	StatusQueued      Status = "queued"
	StatusReading     Status = "reading"
	StatusFinished    Status = "finished"
	StatusAbandoned   Status = "abandoned"
	StatusReference   Status = "reference"
)

// ValidStatuses is ordered for display purposes.
var ValidStatuses = []Status{
	StatusAntilibrary, StatusQueued, StatusReading,
	StatusFinished, StatusAbandoned, StatusReference,
}

// IsValidStatus reports whether s is a known status value.
func IsValidStatus(s Status) bool {
	for _, v := range ValidStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// Date is a YYYY-MM-DD date that marshals to/from YAML as a plain string,
// avoiding the timestamp suffix that time.Time produces.
type Date struct {
	Year, Month, Day int
}

func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

func (d Date) IsZero() bool {
	return d.Year == 0 && d.Month == 0 && d.Day == 0
}

// Before reports whether d is before other.
func (d Date) Before(other Date) bool {
	if d.Year != other.Year {
		return d.Year < other.Year
	}
	if d.Month != other.Month {
		return d.Month < other.Month
	}
	return d.Day < other.Day
}

func (d Date) MarshalYAML() (interface{}, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.String(), nil
}

func (d *Date) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	var y, m, day int
	if _, err := fmt.Sscanf(s, "%d-%d-%d", &y, &m, &day); err != nil {
		return fmt.Errorf("invalid date %q: expected YYYY-MM-DD", s)
	}
	d.Year, d.Month, d.Day = y, m, day
	return nil
}

// Frontmatter mirrors the YAML block of a book file.
type Frontmatter struct {
	Title    string   `yaml:"title"`
	Author   string   `yaml:"author"`
	Status   Status   `yaml:"status"`
	Track    string   `yaml:"track,omitempty"`
	Started  *Date    `yaml:"started,omitempty"`
	Finished *Date    `yaml:"finished,omitempty"`
	Rating   *int     `yaml:"rating,omitempty"`
	Themes   []string `yaml:"themes,omitempty"`
	ISBN     string   `yaml:"isbn,omitempty"`
	Cover    string   `yaml:"cover,omitempty"`
	Acquired *Date    `yaml:"acquired,omitempty"`
	Source   string   `yaml:"source,omitempty"`
}

// Book is the in-memory representation of a single .md file.
type Book struct {
	Slug     string
	FilePath string
	Meta     Frontmatter
	Body     string    // raw markdown body (everything after frontmatter delimiter)
	ModTime  time.Time // file mtime at parse time
}
