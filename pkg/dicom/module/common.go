package module

import (
	"fmt"
	"time"

	"github.com/jpfielding/goxel/pkg/dicom/tag"
)

// Vector3D represents a 3D vector (x, y, z)
type Vector3D struct {
	X, Y, Z float64
}

// Date represents a DICOM Date (DA VR)
type Date struct {
	Year  int
	Month int
	Day   int
}

func (d Date) String() string {
	return fmt.Sprintf("%04d%02d%02d", d.Year, d.Month, d.Day)
}

func NewDate(t time.Time) Date {
	return Date{
		Year:  t.Year(),
		Month: int(t.Month()),
		Day:   t.Day(),
	}
}

// Time represents a DICOM Time (TM VR)
type Time struct {
	Hour   int
	Minute int
	Second int
	Nano   int
}

func (t Time) String() string {
	// Format as HHMMSS.FFFFFF
	return fmt.Sprintf("%02d%02d%02d.%06d", t.Hour, t.Minute, t.Second, t.Nano/1000)
}

func NewTime(t time.Time) Time {
	return Time{
		Hour:   t.Hour(),
		Minute: t.Minute(),
		Second: t.Second(),
		Nano:   t.Nanosecond(),
	}
}

// PersonName represents a DICOM Person Name (PN VR)
type PersonName struct {
	FamilyName string
	GivenName  string
	MiddleName string
	Prefix     string
	Suffix     string
}

func (p PersonName) String() string {
	// DICOM format: Family^Given^Middle^Prefix^Suffix
	return fmt.Sprintf("%s^%s^%s^%s^%s", p.FamilyName, p.GivenName, p.MiddleName, p.Prefix, p.Suffix)
}

// Common module interfaces
type IODModule interface {
	ToTags() []IODElement
}

type IODElement struct {
	Tag   tag.Tag
	Value interface{}
}
