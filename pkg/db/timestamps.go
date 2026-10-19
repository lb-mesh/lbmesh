package db

import "time"

// timestampLayouts are the textual forms SQLite timestamps can take: the
// CURRENT_TIMESTAMP default, and the RFC 3339 forms the driver writes for
// time.Time arguments.
var timestampLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05",
}

// ParseTimestamp converts a scanned DATETIME column value to a time.Time.
// The modernc.org/sqlite driver returns time.Time for DATETIME columns, but
// rows written by other tools may hold plain text, so both are accepted.
// Unrecognised values yield the zero time.
func ParseTimestamp(v any) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t.UTC()
	case string:
		return parseTimestampString(t)
	case []byte:
		return parseTimestampString(string(t))
	}
	return time.Time{}
}

func parseTimestampString(s string) time.Time {
	for _, layout := range timestampLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
