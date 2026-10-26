// JSON shape tests for every AUnrealFeatures.Ares.Models type: each Go model
// (fully populated and freshly constructed) must serialize byte-for-byte like
// the original assembly did. Fixtures: testdata/models.tsv, produced by
// tools/oracle-ares (`dotnet run -- models`) against the built originals.
package models

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

func s(v string) *string { return &v }

func loadFixtures(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open("testdata/models.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		k, v, ok := strings.Cut(line, "\t")
		if ok {
			out[k] = v
		}
	}
	return out
}

func TestModelsMatchOracle(t *testing.T) {
	fx := loadFixtures(t)
	d1 := dotnet.New(2024, 1, 2, 3, 4, 5, dotnet.Utc).AddTicks(1234567)
	d2 := dotnet.New(2023, 12, 31, 23, 59, 59, dotnet.Unspecified)
	cfg := func() map[string]*string { return map[string]*string{"a": s("1"), "b": s("x<y>&'\"")} }
	ban := &BanRequest{UserId: s("u1"), StationId: s("s1"), Expiration: d1, Reason: s("r"), Revoked: true}
	role := &RoleResponse{RoleData: RoleData{RoleId: s("r1"), StationId: s("s1"), RoleName: s("Admin"), RoleDescription: s("")}, Permissions: []*string{s("admin"), s("station:read")}}
	udr := &UserDataResponse{UserData: UserData{UserId: s("u1"), Username: s("Bob"), DiscordId: s("d"), Platform: s("quest"), CreatedAt: d1, LastLogin: d2}, Roles: []*string{s("r1")}, Bans: []*BanRequest{ban}}
	sdr := &ServerDeploymentResponse{DeploymentId: s("dep"), StationId: s("st"), DeploymentName: s("n"), Region: s("NAE"), IpAddress: s("1.2.3.4:7777"), Version: s("22284"), CreatedAt: d1, Online: true, LastEventAt: d2, PlayerCount: 3, Config: cfg()}
	dex := &DeploymentExpanded{DeploymentRequest: DeploymentRequest{IpAddress: s("ip"), DeploymentName: s("dn"), Region: s("r"), Version: s("v"), StationId: s("st")}, CreatedAt: d1, Online: true, LastEvent: d2, PlayerCount: 7, Config: cfg()}
	fsr := &FleetStationResponse{StationId: s("a"), FleetId: s("b"), SessionId: s("c"), StationName: s("d"), Region: s("e"), Ip: s("f"), Version: s("g"), DeploymentCl: s("h"), Created: d1, Online: true, LastEvent: &d2, PlayerCount: 2, Disabled: false, Config: map[string]any{"flag": true, "num": 1.5, "s": "str"}}
	sev := &ServerEventResponse{Index: 1, EventType: s("t"), DeploymentId: s("dep"), EventData: s("{}"), Timestamp: d1}
	sr := &StationResponse{StationId: s("st"), StationName: s("nm"), CreatedAt: d1, Deployments: []*ServerDeploymentResponse{sdr}, Config: cfg()}
	fr := &FleetResponse{FleetId: s("f"), FleetName: s("fn"), Created: d1, Online: true, Stations: []*FleetStationResponse{fsr}, Config: map[string]any{"x": 2.0}}
	page := func(items int) *PageData {
		return &PageData{TotalItems: items, ItemCount: items, PageSize: 10, Page: 1, Pages: 1}
	}
	drp := &DeploymentsResponsePage{StationId: s("st")}
	drp.Page, drp.Items = page(1), []*DeploymentExpanded{dex}
	frp := &FleetResponsePage{}
	frp.Page, frp.Items = page(1), []*FleetResponse{fr}
	sdrp := &ServerDeploymentResponsePage{}
	sdrp.Page, sdrp.Items = page(1), []*ServerDeploymentResponse{sdr}
	serp := Create[*ServerEventResponse, ServerEventResponsePage]([]*ServerEventResponse{sev, sev}, 10, 1)
	srp := Create[*StationResponse, StationResponsePage]([]*StationResponse{sr}, 10, 1)
	urp := Create[*UserDataResponse, UsersResponsePage]([]*UserDataResponse{udr}, 10, 1)

	cases := []struct {
		name        string
		full, empty any
	}{
		{"ApiKeyResponse", &ApiKeyResponse{ApiKey: s("k")}, NewApiKeyResponse()},
		{"AssignPermsBody", &AssignPermsBody{Permissions: []*string{s("a"), s("b")}}, NewAssignPermsBody()},
		{"BanRequest", ban, NewBanRequest()},
		{"BanResponse", &BanResponse{Bans: []*BanRequest{ban}}, NewBanResponse()},
		{"ClientLoginDeployment", &ClientLoginDeployment{DeploymentId: s("d"), StationId: s("s"), StationName: s("sn"), DeploymentName: s("dn"), IpAddress: s("ip"), IsWhitelist: true, IsPublic: true, IsJoinable: true}, NewClientLoginDeployment()},
		{"ClientLoginRequest", &ClientLoginRequest{Nonce: s("n"), PlatformId: s("p"), Username: s("u"), Platform: s("q"), Device: s("d")}, NewClientLoginRequest()},
		{"ClientLoginResponse", &ClientLoginResponse{Success: true, ApiKey: s("k"), OrgScopedId: s("o"), ServerDeployments: []*ClientLoginDeployment{NewClientLoginDeployment()}}, NewClientLoginResponse()},
		{"ClientLoginV2Response", &ClientLoginV2Response{Success: true, ApiKey: "k", OrgScopedId: "o"}, NewClientLoginV2Response()},
		{"CreateDeploymentResponse", &CreateDeploymentResponse{ApiKey: s("k"), DeploymentId: s("d")}, NewCreateDeploymentResponse()},
		{"CreateRoleRequest", &CreateRoleRequest{RoleName: s("n"), RoleDescription: s("d")}, NewCreateRoleRequest()},
		{"CreateStationRequest", &CreateStationRequest{StationName: s("n"), StationId: s("i")}, NewCreateStationRequest()},
		{"CreateStationResponse", &CreateStationResponse{StationId: s("i")}, NewCreateStationResponse()},
		{"DeploymentDbObject", &DeploymentDbObject{DeploymentId: s("d"), StationId: s("s"), DeploymentName: s("n"), IpAddress: s("ip"), Region: s("r"), Version: s("v"), CreatedAt: d1, Online: true, LastEvent: &d2, PlayerCount: 4, Config: cfg(), ApiKeys: []string{"k"}}, NewDeploymentDbObject()},
		{"DeploymentRequest", &DeploymentRequest{IpAddress: s("ip"), DeploymentName: s("dn"), Region: s("r"), Version: s("v"), StationId: s("s")}, NewDeploymentRequest()},
		{"DeploymentExpanded", dex, NewDeploymentExpanded()},
		{"DeploymentsResponsePage", drp, NewDeploymentsResponsePage()},
		{"EventSignupDbObject", &EventSignupDbObject{SignupId: s("s"), EventId: s("e"), UserId: s("u"), Timestamp: d1, Data: s("x")}, NewEventSignupDbObject()},
		{"EventSignupResponse", &EventSignupResponse{UserSignedUp: true, SignupId: s("s")}, NewEventSignupResponse()},
		{"FleetResponse", fr, NewFleetResponse()},
		{"FleetResponsePage", frp, NewFleetResponsePage()},
		{"FleetStationResponse", fsr, NewFleetStationResponse()},
		{"ValidationError", &ValidationError{Locations: []string{"body"}, Message: s("m"), Type: s("t")}, NewValidationError()},
		{"HTTPValidationError", &HTTPValidationError{Details: []*ValidationError{NewValidationError()}}, NewHTTPValidationError()},
		{"LogInServerRequest", &LogInServerRequest{UserId: s("u"), Username: s("n"), Platform: s("p"), Device: s("d")}, NewLogInServerRequest()},
		{"UserPermission", &UserPermission{StationId: s("s"), Permission: s("p")}, NewUserPermission()},
		{"LogInServerResponse", &LogInServerResponse{ApiKey: s("k"), User: &udr.UserData, Permissions: []*UserPermission{{StationId: s("global"), Permission: s("admin")}}}, NewLogInServerResponse()},
		{"PageData", &PageData{TotalItems: 1, ItemCount: 2, PageSize: 3, Page: 4, Pages: 5}, NewPageData()},
		{"RegionResponse", &RegionResponse{ContinentCode: "EU", CountryCode: "DE"}, NewRegionResponse()},
		{"RegisterServerRequest", &RegisterServerRequest{Ip: s("1"), Port: s("2"), DeploymentId: s("3"), ServerName: s("4"), Region: s("5"), ImguiPort: s("6"), MaxPlayers: 7}, NewRegisterServerRequest()},
		{"RegisterServerResponse", &RegisterServerResponse{Success: true, StationId: s("s"), DeploymentId: s("d")}, NewRegisterServerResponse()},
		{"UpdatePlayerCountRequest", &UpdatePlayerCountRequest{DeploymentId: s("d"), PlayerCount: 3}, NewUpdatePlayerCountRequest()},
		{"ServerHeartbeatRequest", &ServerHeartbeatRequest{DeploymentId: s("d"), Pid: 1, Seq: 18446744073709551615, DispatchAgeMs: 2, TickAgeMs: 3, UptimeMs: 4, Players: 5}, NewServerHeartbeatRequest()},
		{"RoleData", &RoleData{RoleId: s("r"), StationId: s("s"), RoleName: s("n"), RoleDescription: s("d")}, NewRoleData()},
		{"RoleResponse", role, NewRoleResponse()},
		{"RolesResponse", &RolesResponse{Roles: []*RoleResponse{role}}, NewRolesResponse()},
		{"ServerDeploymentResponse", sdr, NewServerDeploymentResponse()},
		{"ServerDeploymentResponsePage", sdrp, NewServerDeploymentResponsePage()},
		{"ServerEventDbObject", &ServerEventDbObject{DeploymentId: s("d"), EventType: s("t"), EventData: s("x"), Timestamp: d2}, NewServerEventDbObject()},
		{"ServerEventRequest", &ServerEventRequest{EventType: s("t"), EventData: s("x")}, NewServerEventRequest()},
		{"ServerEventResponse", sev, NewServerEventResponse()},
		{"ServerEventResponsePage", serp, NewServerEventResponsePage()},
		{"StationDbObject", &StationDbObject{StationId: s("s"), StationName: s("n"), CreatedAt: d1, Online: true, LastOnline: &d2, Config: cfg()}, NewStationDbObject()},
		{"StationDeploymentResponse", &StationDeploymentResponse{StationId: s("s"), Deployments: []*DeploymentExpanded{dex}}, NewStationDeploymentResponse()},
		{"StationEventDbObject", &StationEventDbObject{EventId: s("e"), StationId: s("s"), DeploymentId: s("d"), Title: s("t"), Description: s("de"), StartTime: d1, Duration: 60, Public: true, SignupsOpen: true, Config: cfg()}, NewStationEventDbObject()},
		{"StationEventSignupInfo", &StationEventSignupInfo{SignupId: s("s"), UserId: s("u"), Timestamp: d1, Data: s("x")}, NewStationEventSignupInfo()},
		{"StationEventSignups", &StationEventSignups{Signups: []*StationEventSignupInfo{NewStationEventSignupInfo()}}, NewStationEventSignups()},
		{"StationEvent", &StationEvent{EventId: s("e"), Title: s("t"), Description: s("d"), StartTime: d1, Duration: 5, Public: true, SignupsOpen: true, StationId: s("s"), DeploymentId: s("")}, NewStationEvent()},
		{"StationEvents", &StationEvents{Events: []*StationEvent{NewStationEvent()}}, NewStationEvents()},
		{"StationPerm", &StationPerm{StationId: s("s"), Permission: s("p")}, NewStationPerm()},
		{"StationResponse", sr, NewStationResponse()},
		{"StationResponsePage", srp, NewStationResponsePage()},
		{"StationsPageV2", &StationsPageV2{FleetId: "", Items: []*FleetStationResponse{fsr}}, NewStationsPageV2()},
		{"SuccessBoolean", &SuccessBoolean{Success: true}, NewSuccessBoolean()},
		{"UpdateStationRequest", &UpdateStationRequest{StationName: s("n")}, NewUpdateStationRequest()},
		{"UpdateStationResponse", &UpdateStationResponse{StationName: s("n")}, NewUpdateStationResponse()},
		{"UserApiKeyDbObject", &UserApiKeyDbObject{UserId: s("u"), ApiKey: s("k")}, NewUserApiKeyDbObject()},
		{"UserData", &UserData{UserId: s("u"), Username: s("n"), DiscordId: s("d"), Platform: s("p"), CreatedAt: d1, LastLogin: d2}, NewUserData()},
		{"UserDataResponse", udr, NewUserDataResponse()},
		{"UserDataV1Response", FromStorage(udr, []*RoleResponse{role}, true), NewUserDataV1Response()},
		{"UserDataV1ResponseNoBans", FromStorage(udr, nil, false), NewUserDataV1Response()},
		{"UserRequest", &UserRequest{UserId: s("u"), Username: s("n"), DiscordId: s("d"), Platform: s("p")}, NewUserRequest()},
		{"UserRoleByNameRequest", &UserRoleByNameRequest{RoleId: s("r"), Username: s("u"), ExpiresInHours: 3}, NewUserRoleByNameRequest()},
		{"UserRoleByNameResponse", &UserRoleByNameResponse{Success: true, UserExists: true}, NewUserRoleByNameResponse()},
		{"UsersResponse", &UsersResponse{Users: []*UserDataResponse{udr}}, NewUsersResponse()},
		{"UsersResponsePage", urp, NewUsersResponsePage()},
	}
	seen := map[string]bool{}
	for _, c := range cases {
		for _, v := range []struct {
			key string
			obj any
		}{{c.name, c.full}, {c.name + ".empty", c.empty}} {
			want, ok := fx[v.key]
			if !ok {
				t.Errorf("%s: no oracle fixture", v.key)
				continue
			}
			seen[v.key] = true
			if got := stj.MarshalString(v.obj); got != want {
				t.Errorf("%s:\n got  %s\n want %s", v.key, got, want)
			}
		}
	}
	// Every model fixture the oracle emitted must be covered (DrafterGrant and
	// SpecEntry live in package ares and are checked there).
	for k := range fx {
		if strings.HasPrefix(k, "Create.") || strings.HasPrefix(k, "DrafterGrant") || strings.HasPrefix(k, "SpecEntry") {
			continue
		}
		if !seen[k] {
			t.Errorf("oracle fixture %s not covered", k)
		}
	}
}

// TestCreatePagingMatchesOracle checks PagedResponse<T>.Create arithmetic
// (page clamping, skip/take, page counts) against the original.
func TestCreatePagingMatchesOracle(t *testing.T) {
	fx := loadFixtures(t)
	d2 := dotnet.New(2023, 12, 31, 23, 59, 59, dotnet.Unspecified)
	items := []*ServerEventResponse{}
	for i := 1; i <= 7; i++ {
		items = append(items, &ServerEventResponse{Index: i, Timestamp: d2})
	}
	for _, pc := range [][2]int{{3, 1}, {3, 3}, {3, 9}, {100, 1}, {7, 2}, {1, 7}} {
		key := "Create." + itoa(pc[0]) + "." + itoa(pc[1])
		got := stj.MarshalString(Create[*ServerEventResponse, ServerEventResponsePage](items, pc[0], pc[1]))
		if got != fx[key] {
			t.Errorf("%s:\n got  %s\n want %s", key, got, fx[key])
		}
	}
	got := stj.MarshalString(Create[*ServerEventResponse, ServerEventResponsePage]([]*ServerEventResponse{}, 10, 5))
	if got != fx["Create.empty"] {
		t.Errorf("Create.empty:\n got  %s\n want %s", got, fx["Create.empty"])
	}
}

func TestCreateThrows(t *testing.T) {
	for _, c := range []struct {
		items     []int
		size, pag int
		typ       string
	}{{nil, 1, 1, "System.ArgumentNullException"}, {[]int{}, 0, 1, "System.ArgumentException"}, {[]int{}, 1, 0, "System.ArgumentException"}} {
		func() {
			defer func() {
				p := recover()
				if e, ok := p.(*dotnet.Exception); !ok || e.Type != c.typ {
					t.Errorf("want %s, got %v", c.typ, p)
				}
			}()
			type page struct{ PagedResponse[int] }
			Create[int, page](c.items, c.size, c.pag)
		}()
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
