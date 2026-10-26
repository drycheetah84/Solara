package stj

import (
	"strings"
	"testing"

	"solara/internal/dotnet"
)

// u expands the "%u" placeholder to a JSON unicode escape introducer. (The
// placeholder keeps literal escape sequences out of the source text.)
func u(s string) string { return strings.ReplaceAll(s, "%u", "\\"+"u") }

// Expected strings below were produced by the original runtime (.NET 6
// System.Text.Json) via tools/oracle; see VALIDATION_REPORT.md.
type oracleM struct {
	S   string
	D1  float64
	D2  float64
	D3  float64
	D4  float64
	D5  float64
	F1  float32
	F2  float32
	D6  float64
	D7  float64
	U   dotnet.DateTime
	N   dotnet.DateTime
	B   []byte
	Big int64
}

func TestOracleSerialization(t *testing.T) {
	m := oracleM{
		S:  "a<b>&'+`" + "\xc3\xa9" + string(rune(0x1F600)) + "\x01\n\"\\/",
		D1: 1e15, D2: 123456789012345, D3: 0.00001, D4: 0.0001, D5: 5,
		F1: 1e7, F2: 0.1, D6: 0.1, D7: 1.0 / 3,
		U:   dotnet.New(2025, 1, 2, 3, 4, 5, dotnet.Utc).AddTicks(1234500),
		N:   dotnet.New(2025, 1, 2, 3, 4, 5, dotnet.Unspecified),
		B:   []byte{1, 2, 3},
		Big: 9223372036854775807,
	}
	want := u(`{"S":"a%u003Cb%u003E%u0026%u0027%u002B%u0060%u00E9%uD83D%uDE00%u0001\n%u0022\\/","D1":1000000000000000,"D2":123456789012345,"D3":1E-05,"D4":0.0001,"D5":5,"F1":10000000,"F2":0.1,"D6":0.1,"D7":0.3333333333333333,"U":"2025-01-02T03:04:05.12345Z","N":"2025-01-02T03:04:05","B":"AQID","Big":9223372036854775807}`)
	if got := MarshalString(m); got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

func TestOracleDoubles(t *testing.T) {
	cases := map[float64]string{1e15: "1000000000000000", 1e16: "10000000000000000", 1.5e16: "15000000000000000",
		123456789012345678: "1.2345678901234568E+17", 1e20: "1E+20", 1e21: "1E+21", 1e-4: "0.0001", 1.234e-5: "1.234E-05",
		5e-324: "5E-324", 1.7976931348623157e308: "1.7976931348623157E+308", 12345.678: "12345.678"}
	for in, want := range cases {
		if got := dotnet.FormatDouble(in); got != want {
			t.Errorf("%v: got %s want %s", in, got, want)
		}
	}
	fc := map[float32]string{1e7: "10000000", 1e8: "100000000", 3.4e38: "3.4E+38", 1.5e-5: "1.5E-05", 16777217: "16777216", 0.3: "0.3"}
	for in, want := range fc {
		if got := dotnet.FormatSingle(in); got != want {
			t.Errorf("%v: got %s want %s", in, got, want)
		}
	}
	want := u(`"%u2028%u007F%u00A0\t\r\b\f%u0000"`)
	if got := MarshalString(string([]rune{0x2028, 0x7f, 0xa0, 9, 13, 8, 12, 0})); got != want {
		t.Errorf("control escapes: %s", got)
	}
}

func TestElementToString(t *testing.T) {
	el := MustParse([]byte(`{"a":true,"b":1.50,"c":"x","d":null,"e":{"z":1},"f":[1, 2]}`))
	want := map[string]string{"a": "True", "b": "1.50", "c": "x", "d": "", "e": `{"z":1}`, "f": "[1, 2]"}
	for _, p := range el.EnumerateObject() {
		if p.Value.ToString() != want[p.Name] {
			t.Errorf("%s: got %q want %q", p.Name, p.Value.ToString(), want[p.Name])
		}
	}
}

func TestStrictDecode(t *testing.T) {
	var m struct {
		D1 float64
		S  string
	}
	if err := Unmarshal([]byte(`{"D1":null}`), &m); err == nil {
		t.Error("null into double must fail")
	}
	if err := Unmarshal([]byte(`{"S":5}`), &m); err == nil {
		t.Error("number into string must fail")
	}
	m.S = "keep"
	if err := Unmarshal([]byte(`{"s":"lower"}`), &m); err != nil || m.S != "keep" {
		t.Error("matching must be case-sensitive")
	}
	var d dotnet.DateTime
	if err := Unmarshal([]byte(`"2025-01-02T03:04:05+02:00"`), &d); err != nil || d.Kind() != dotnet.Local {
		t.Error("offset date must become Local")
	}
	if e := MustParse([]byte(`"` + u("%uD83D%uDE00") + `"`)); e.Str() != string(rune(0x1F600)) {
		t.Error("surrogate pair decode")
	}
}

// JsonElement re-serialization is compact and re-escaped (verified against .NET by the EOS oracle).
func TestElementMarshalCompact(t *testing.T) {
	el := MustParse([]byte(`{ "a" : [1, 2.50 ,"<x>"], "b":{} }`))
	want := u(`{"a":[1,2.50,"%u003Cx%u003E"],"b":{}}`)
	if got := MarshalString(el); got != want {
		t.Errorf("got %s want %s", got, want)
	}
}
