// Package dotnet reproduces the externally observable semantics of the .NET
// primitives the original Halcyon backend relied on (System.DateTime, Guid,
// number formatting, LiteDB ObjectId), so that Solara's JSON output and
// persisted data match the original byte-for-byte where it matters.
package dotnet

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Kind mirrors System.DateTimeKind.
type Kind uint8

const (
	Unspecified Kind = 0
	Utc         Kind = 1
	Local       Kind = 2
)

// Ticks per unit, as in System.TimeSpan.
const (
	TicksPerMillisecond int64 = 10_000
	TicksPerSecond            = TicksPerMillisecond * 1000
	TicksPerMinute            = TicksPerSecond * 60
	TicksPerHour              = TicksPerMinute * 60
	TicksPerDay               = TicksPerHour * 24
	maxTicks                  = 3155378975999999999
	// ticks between 0001-01-01 and 1970-01-01
	unixEpochTicks int64 = 621355968000000000
)

// DateTime mirrors System.DateTime: a wall-clock tick count (100ns since
// 0001-01-01) plus a Kind. Like .NET, comparisons and arithmetic operate on
// the raw ticks and ignore Kind; that is observable behaviour the original
// code depends on (e.g. comparing a LiteDB-loaded Local value with UtcNow).
type DateTime struct {
	ticks int64
	kind  Kind
}

var (
	MinValue = DateTime{0, Unspecified}
	MaxValue = DateTime{maxTicks, Unspecified}
)

// FromTicks builds a DateTime from raw ticks and kind.
func FromTicks(ticks int64, kind Kind) DateTime { return DateTime{ticks, kind} }

// New mirrors new DateTime(y, mo, d, h, mi, s, kind).
func New(year, month, day, hour, min, sec int, kind Kind) DateTime {
	t := time.Date(year, time.Month(month), day, hour, min, sec, 0, time.UTC)
	return DateTime{wallTicks(t), kind}
}

func wallTicks(t time.Time) int64 {
	// t interpreted as wall clock in its own location
	y, mo, d := t.Date()
	h, mi, s := t.Clock()
	u := time.Date(y, mo, d, h, mi, s, t.Nanosecond(), time.UTC)
	return u.Unix()*TicksPerSecond + int64(u.Nanosecond())/100 + unixEpochTicks
}

// FromTime converts a Go time to a DateTime of the requested kind, taking the
// wall clock of t in UTC (kind Utc), the local zone (Local) or t's own
// location (Unspecified).
func FromTime(t time.Time, kind Kind) DateTime {
	switch kind {
	case Utc:
		t = t.UTC()
	case Local:
		t = t.Local()
	}
	return DateTime{wallTicks(t), kind}
}

// UtcNow mirrors DateTime.UtcNow.
func UtcNow() DateTime { return FromTime(time.Now(), Utc) }

// Now mirrors DateTime.Now.
func Now() DateTime { return FromTime(time.Now(), Local) }

func (d DateTime) Ticks() int64 { return d.ticks }
func (d DateTime) Kind() Kind   { return d.kind }
func (d DateTime) IsZero() bool { return d.ticks == 0 }

// wall returns the wall clock as a UTC-located time.Time.
func (d DateTime) wall() time.Time {
	t := d.ticks - unixEpochTicks
	sec := t / TicksPerSecond
	rem := t % TicksPerSecond
	if rem < 0 {
		rem += TicksPerSecond
		sec--
	}
	return time.Unix(sec, rem*100).UTC()
}

// Time returns the instant this DateTime denotes: Utc and Unspecified wall
// clocks are read as UTC, Local as the machine's local zone.
func (d DateTime) Time() time.Time {
	w := d.wall()
	if d.kind == Local {
		return time.Date(w.Year(), w.Month(), w.Day(), w.Hour(), w.Minute(), w.Second(), w.Nanosecond(), time.Local)
	}
	return w
}

// ToUniversalTime mirrors DateTime.ToUniversalTime (Unspecified is treated as local).
func (d DateTime) ToUniversalTime() DateTime {
	if d.kind == Utc {
		return d
	}
	if d.ticks == 0 || d.ticks == maxTicks {
		return DateTime{d.ticks, Utc}
	}
	w := d.wall()
	lt := time.Date(w.Year(), w.Month(), w.Day(), w.Hour(), w.Minute(), w.Second(), w.Nanosecond(), time.Local)
	return FromTime(lt, Utc)
}

// ToLocalTime mirrors DateTime.ToLocalTime (Unspecified is treated as UTC).
func (d DateTime) ToLocalTime() DateTime {
	if d.kind == Local {
		return d
	}
	return FromTime(d.wall(), Local)
}

func (d DateTime) clamp(t int64) DateTime {
	if t < 0 || t > maxTicks {
		panic(&Exception{Type: "System.ArgumentOutOfRangeException", Message: "The added or subtracted value results in an un-representable DateTime. (Parameter 'value')"})
	}
	return DateTime{t, d.kind}
}

func (d DateTime) AddTicks(t int64) DateTime      { return d.clamp(d.ticks + t) }
func (d DateTime) Add(dur time.Duration) DateTime { return d.clamp(d.ticks + int64(dur/100)) }
func (d DateTime) AddMilliseconds(v float64) DateTime {
	return d.clamp(d.ticks + roundTicks(v, TicksPerMillisecond))
}
func (d DateTime) AddSeconds(v float64) DateTime {
	return d.clamp(d.ticks + roundTicks(v, TicksPerSecond))
}
func (d DateTime) AddMinutes(v float64) DateTime {
	return d.clamp(d.ticks + roundTicks(v, TicksPerMinute))
}
func (d DateTime) AddHours(v float64) DateTime { return d.clamp(d.ticks + roundTicks(v, TicksPerHour)) }
func (d DateTime) AddDays(v float64) DateTime  { return d.clamp(d.ticks + roundTicks(v, TicksPerDay)) }

// roundTicks reproduces .NET 6 DateTime.Add(double, scale): value*scale rounded
// to the nearest millisecond (.NET 6 semantics).
func roundTicks(v float64, scale int64) int64 {
	half := 0.5
	if v < 0 {
		half = -0.5
	}
	return int64(v*float64(scale/TicksPerMillisecond)+half) * TicksPerMillisecond
}

// Sub mirrors (a - b) as a TimeSpan.
func (d DateTime) Sub(o DateTime) time.Duration { return time.Duration((d.ticks - o.ticks) * 100) }

func (d DateTime) Before(o DateTime) bool { return d.ticks < o.ticks }
func (d DateTime) After(o DateTime) bool  { return d.ticks > o.ticks }
func (d DateTime) Equal(o DateTime) bool  { return d.ticks == o.ticks }
func (d DateTime) Compare(o DateTime) int {
	switch {
	case d.ticks < o.ticks:
		return -1
	case d.ticks > o.ticks:
		return 1
	}
	return 0
}

// Date mirrors DateTime.Date.
func (d DateTime) Date() DateTime { return DateTime{d.ticks - d.ticks%TicksPerDay, d.kind} }

func (d DateTime) Year() int   { return d.wall().Year() }
func (d DateTime) Month() int  { return int(d.wall().Month()) }
func (d DateTime) Day() int    { return d.wall().Day() }
func (d DateTime) Hour() int   { return d.wall().Hour() }
func (d DateTime) Minute() int { return d.wall().Minute() }
func (d DateTime) Second() int { return d.wall().Second() }

// ToUnixMilliseconds mirrors new DateTimeOffset(d).ToUnixTimeMilliseconds().
func (d DateTime) ToUnixMilliseconds() int64 { return d.Time().UnixMilli() }

// ToUnixSeconds mirrors new DateTimeOffset(d).ToUnixTimeSeconds().
func (d DateTime) ToUnixSeconds() int64 { return d.Time().Unix() }

func (d DateTime) fraction() int64 { return d.ticks % TicksPerSecond }

func (d DateTime) offsetSuffix() string {
	switch d.kind {
	case Utc:
		return "Z"
	case Local:
		_, off := d.Time().Zone()
		s := "+"
		if off < 0 {
			s = "-"
			off = -off
		}
		return fmt.Sprintf("%s%02d:%02d", s, off/3600, (off%3600)/60)
	}
	return ""
}

// FormatSTJ renders the value as System.Text.Json writes a DateTime:
// ISO-8601 with up to seven fractional digits, trailing zeros trimmed.
func (d DateTime) FormatSTJ() string {
	w := d.wall()
	s := w.Format("2006-01-02T15:04:05")
	if f := d.fraction(); f != 0 {
		s += "." + strings.TrimRight(fmt.Sprintf("%07d", f), "0")
	}
	return s + d.offsetSuffix()
}

// FormatO mirrors DateTime.ToString("o") (always seven fractional digits).
func (d DateTime) FormatO() string {
	return d.wall().Format("2006-01-02T15:04:05") + fmt.Sprintf(".%07d", d.fraction()) + d.offsetSuffix()
}

// Format renders a .NET custom format string. Supported specifiers cover what
// the original code uses: yyyy MM dd HH mm ss fff and literal characters.
func (d DateTime) Format(layout string) string {
	w := d.wall()
	r := strings.NewReplacer(
		"yyyy", fmt.Sprintf("%04d", w.Year()),
		"MM", fmt.Sprintf("%02d", int(w.Month())),
		"dd", fmt.Sprintf("%02d", w.Day()),
		"HH", fmt.Sprintf("%02d", w.Hour()),
		"mm", fmt.Sprintf("%02d", w.Minute()),
		"ss", fmt.Sprintf("%02d", w.Second()),
		"fff", fmt.Sprintf("%03d", d.fraction()/TicksPerMillisecond),
	)
	return r.Replace(layout)
}

// String mirrors DateTime.ToString() under the invariant culture
// (the deployed backend runs with DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1).
func (d DateTime) String() string {
	w := d.wall()
	return fmt.Sprintf("%02d/%02d/%04d %02d:%02d:%02d", int(w.Month()), w.Day(), w.Year(), w.Hour(), w.Minute(), w.Second())
}

// MarshalSTJ implements stj.Marshaler.
func (d DateTime) MarshalSTJ() ([]byte, error) { return []byte(strconv.Quote(d.FormatSTJ())), nil }

// ParseSTJ parses an ISO-8601 string the way System.Text.Json reads a
// DateTime: "Z" yields Utc, an explicit offset is converted to Local, no
// designator yields Unspecified.
func ParseSTJ(s string) (DateTime, error) {
	layouts := []struct {
		l    string
		zone bool
	}{
		{"2006-01-02T15:04:05.9999999Z07:00", true},
		{"2006-01-02T15:04:05Z07:00", true},
		{"2006-01-02T15:04Z07:00", true},
		{"2006-01-02T15:04:05.9999999", false},
		{"2006-01-02T15:04:05", false},
		{"2006-01-02T15:04", false},
		{"2006-01-02", false},
	}
	for _, l := range layouts {
		t, err := time.Parse(l.l, s)
		if err != nil {
			continue
		}
		if !l.zone {
			return DateTime{wallTicks(t), Unspecified}, nil
		}
		if strings.HasSuffix(s, "Z") || strings.HasSuffix(s, "z") {
			return DateTime{wallTicks(t.UTC()), Utc}, nil
		}
		return FromTime(t, Local), nil
	}
	return DateTime{}, &Exception{Type: "System.Text.Json.JsonException", Message: "The JSON value could not be converted to System.DateTime."}
}

// FromUnixMilliseconds mirrors DateTimeOffset.FromUnixTimeMilliseconds(ms).UtcDateTime.
func FromUnixMilliseconds(ms int64) DateTime {
	return DateTime{ms*TicksPerMillisecond + unixEpochTicks, Utc}
}

// FromUnixSeconds mirrors DateTimeOffset.FromUnixTimeSeconds(s).UtcDateTime.
func FromUnixSeconds(s int64) DateTime { return DateTime{s*TicksPerSecond + unixEpochTicks, Utc} }
