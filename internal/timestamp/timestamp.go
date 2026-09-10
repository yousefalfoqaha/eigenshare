package timestamp

import (
	"database/sql/driver"
	"fmt"
	"time"
)

type Timestamp time.Time

func (t Timestamp) String() string         { return time.Time(t).UTC().Format(time.RFC3339) }
func (t Timestamp) Time() time.Time        { return time.Time(t) }
func (t Timestamp) IsZero() bool           { return time.Time(t).IsZero() }
func (t Timestamp) Equal(u Timestamp) bool { return time.Time(t).Equal(time.Time(u)) }

func Now() Timestamp { return Timestamp(time.Now().UTC()) }

func (t Timestamp) MarshalJSON() ([]byte, error) { return time.Time(t).UTC().MarshalJSON() }
func (t *Timestamp) UnmarshalJSON(b []byte) error {
	var tt time.Time
	if err := tt.UnmarshalJSON(b); err != nil {
		return err
	}

	*t = Timestamp(tt.UTC())

	return nil
}

func (t Timestamp) Value() (driver.Value, error) {
	return time.Time(t).UTC().Format(time.RFC3339Nano), nil
}

func (t *Timestamp) Scan(src any) error {
	str, ok := src.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", src)
	}

	parsed, err := time.Parse(time.RFC3339Nano, str)
	if err != nil {
		return err
	}

	*t = Timestamp(parsed.UTC())

	return nil
}
