package halcyonsocket

import (
	"sort"
	"strconv"
	"strings"
	"testing"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

// Expected strings were produced by the ORIGINAL assemblies under .NET 6
// (tools/oracle-halcyonsocket, modes "dto" and "misc").

func TestDTOJsonShapes(t *testing.T) {
	dt := dotnet.New(2026, 10, 25, 12, 34, 56, dotnet.Utc).AddTicks(789*dotnet.TicksPerMillisecond + 1234)
	full := &WatchdogRow{Deployment_id: "d<1>", Name: "N" + string(rune(0xE9)), Box: "b", Pid: 7, Players: -1, Uptime_minutes: 3,
		Last_heartbeat: &dt, Dispatch_age_ms: ^uint64(0), Tick_age_ms: 5, Verdict: "v", Restarts: 2, Last_restart: &dt,
		Retired: true, Retired_reason: "r"}
	cases := []struct {
		v    any
		want string
	}{
		{NewWatchdogRow(), `{"deployment_id":"","name":"","box":"","pid":0,"players":0,"uptime_minutes":0,"last_heartbeat":null,"dispatch_age_ms":0,"tick_age_ms":0,"verdict":"","restarts":0,"last_restart":null,"retired":false,"retired_reason":""}`},
		{full, u(`{"deployment_id":"d%u003C1%u003E","name":"N%u00E9","box":"b","pid":7,"players":-1,"uptime_minutes":3,"last_heartbeat":"2026-10-25T12:34:56.7891234Z","dispatch_age_ms":18446744073709551615,"tick_age_ms":5,"verdict":"v","restarts":2,"last_restart":"2026-10-25T12:34:56.7891234Z","retired":true,"retired_reason":"r"}`)},
		{NewWatchdogStatusResult(), `{"servers":[]}`},
		{NewSpinUpResult(), `{"success":false,"request_id":"","agent":"","error":""}`},
		{NewSpinUpBody(), `{"box":null,"map":null,"region":null,"args":null,"name":null}`},
		{NewWatchdogBody(), `{"deployment_id":null,"reason":null}`},
		{NewSuccessResult(), `{"success":false}`},
		{&AgentInfo{Box: "x", Capacity: 2, Running: 1, Free: 1, Last_seen: dt}, `{"box":"x","capacity":2,"running":1,"free":1,"last_seen":"2026-10-25T12:34:56.7891234Z"}`},
		{NewAgentListResult(), `{"agents":[]}`},
		// the wire payloads (anonymous objects in the original)
		{stj.O("type", "spinup", "id", "abc", "map", "M&<'>", "region", string([]rune{0xFC, '+', 0x4E2D}), "args", "-a=\"q\" \\ \t"),
			u(`{"type":"spinup","id":"abc","map":"M%u0026%u003C%u0027%u003E","region":"%u00FC%u002B%u4E2D","args":"-a=%u0022q%u0022 \\ \t"}`)},
		{stj.O("type", "kill", "pid", "12", "reason", "engine-frozen"), `{"type":"kill","pid":"12","reason":"engine-frozen"}`},
	}
	for i, c := range cases {
		if got := stj.MarshalString(c.v); got != c.want {
			t.Errorf("case %d:\n got %s\nwant %s", i, got, c.want)
		}
	}
}

func TestSpinUpBodyDeserialize(t *testing.T) {
	cases := map[string]string{
		"":                                    "EX",
		"null":                                "null",
		"{}":                                  "box=<null> map=<null>",
		"[]":                                  "EX",
		`{"box":5}`:                           "EX",
		`{"Box":"x","box":"y","map":null}`:    "box=y map=<null>",
		`{"deployment_id":"a","reason":null}`: "box=<null> map=<null>",
	}
	or := func(s *string) string {
		if s == nil {
			return "<null>"
		}
		return *s
	}
	for in, want := range cases {
		var b *SpinUpBody
		got := "EX"
		if tryDeserialize([]byte(in), &b) {
			if b == nil {
				got = "null"
			} else {
				got = "box=" + or(b.Box) + " map=" + or(b.Map)
			}
		}
		if got != want {
			t.Errorf("%q: got %s want %s", in, got, want)
		}
	}
}

func TestDeploymentIdRegex(t *testing.T) {
	r := func(x int) string { return string(rune(x)) }
	cases := []struct{ in, want string }{
		{"-x -DashboardDeploymentId=abc -y", "abc"},
		{"-dashboarddeploymentid=Q1" + r(0xA0) + "tail", "Q1"},
		{"-DASHBOARDDEPLOYMENTID=" + r(0x2003) + "x", "<nomatch>"},
		{"-DashboardDeploymentId=a" + r(0x85) + "b", "a"},
		{"-Dashboard" + r(0x17F) + "DeploymentId=no", "<nomatch>"},
		{"-DashboardDeploymentId=", "<nomatch>"},
		{"-DashboardDeploymentId=one -DashboardDeploymentId=two", "one"},
		{"-DashboardDeploymentId=x" + r(0x200B) + "y", "x<200B>y"},
	}
	for _, c := range cases {
		got := "<nomatch>"
		if m := deploymentIdRe.FindStringSubmatch(c.in); m != nil {
			got = esc(m[1])
		}
		if got != c.want {
			t.Errorf("%s: got %s want %s", esc(c.in), got, c.want)
		}
	}
}

func TestNetCompatHelpers(t *testing.T) {
	r := func(x int) string { return string(rune(x)) }
	ens := map[string]string{
		"":                         "-DashboardDeploymentId=DEP",
		"  -a  ":                   "-a   -DashboardDeploymentId=DEP",
		"-dashboarddeploymentid=x": "-dashboarddeploymentid=x",
		"-Dashboard" + r(0x130) + "DeploymentId=x": "-Dashboard<0130>DeploymentId=x -DashboardDeploymentId=DEP",
	}
	for in, want := range ens {
		if got := esc(EnsureDeploymentArg(in, "DEP")); got != want {
			t.Errorf("Ensure(%q) = %s want %s", in, got, want)
		}
	}
	parse := map[string]string{"12": "12", " +12 ": "12", "-0": "0", "1\x00\x00": "1", "1 \x00": "1", "2147483648": "fail",
		"-2147483648": "-2147483648", "": "fail", "1.0": "fail", r(0x661): "fail", "\t7\n": "7"}
	for in, want := range parse {
		got := "fail"
		if v, ok := parseInt32(in); ok {
			got = itoa(v)
		}
		if got != want {
			t.Errorf("TryParse(%q) = %s want %s", in, got, want)
		}
	}
	f0 := map[float64]string{0.5: "0", 1.5: "2", 2.5: "2", -0.4: "-0", 179.9996: "180", 12345.5: "12346"}
	for in, want := range f0 {
		if got := formatF0(in); got != want {
			t.Errorf("F0(%v) = %s want %s", in, got, want)
		}
	}
	names := []string{"b", "B", "a", "", string([]rune{0x1F600}), r(0xFF21), "aa", "Z"}
	sort.SliceStable(names, func(i, j int) bool { return compareOrdinal(names[i], names[j]) < 0 })
	var e []string
	for _, n := range names {
		e = append(e, esc(n))
	}
	if got := strings.Join(e, "|"); got != "|B|Z|a|aa|b|<D83D><DE00>|<FF21>" {
		t.Errorf("order: %s", got)
	}
	if got := esc(trim(" " + r(0xA0) + r(0x3000) + "x" + r(0x2028) + "\t ")); got != "x" {
		t.Errorf("trim: %s", got)
	}
	shorts := map[string]string{"abc": "abc", "0123456789": "01234567", "abcdefg" + string([]rune{0x1F600}) + "z": "abcdefg<FFFD>"}
	for in, want := range shorts {
		// the original yields a lone high surrogate, which the UTF-8 console writes as U+FFFD
		if got := esc(Short(in)); got != want {
			t.Errorf("Short(%q) = %s want %s", in, got, want)
		}
	}
	// Encoding.UTF8.GetString replacement behaviour (oracle "hub" mode)
	if got := esc(decodeUTF8([]byte{0xE2, 0x82, 0x41, 0xF0, 0x9F, 0x80, 0xC0})); got != "<FFFD>A<FFFD><FFFD>" {
		t.Errorf("decode: %s", got)
	}
	if got := esc(decodeUTF8([]byte("caf\xc3")) + decodeUTF8([]byte("\xa9"))); got != "caf<FFFD><FFFD>" {
		t.Errorf("decode split: %s", got)
	}
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func TestPendingNames(t *testing.T) {
	PendingNames.Set("d1", "Name")
	if v, ok := PendingNames.TryRemove("d1"); !ok || v != "Name" {
		t.Fatal("TryRemove")
	}
	if _, ok := PendingNames.TryRemove("d1"); ok {
		t.Fatal("second TryRemove")
	}
}
