package ares

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"solara/internal/ares/models"
	"solara/internal/docdb"
)

func TestDashboardUserGlobalPermissions(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	seedUsers(t, 1) // user 1000000 with an unrelated role "keep-me"
	s := newAresDashboardServer()
	uid := "1000000"
	user := func() *models.UserDataResponse {
		_, u := dashFindUserRow(uid)
		return u
	}
	type state struct {
		Permissions []string `json:"permissions"`
		Known       []string `json:"known"`
	}
	read := func(code int, body []byte) state {
		t.Helper()
		var st state
		if err := json.Unmarshal(body, &st); err != nil || code != 200 {
			t.Fatalf("state: %d %v %s", code, err, body)
		}
		return st
	}
	// effective permissions the station dashboard hands the game (station -> perms)
	effective := func() map[string][]string {
		out := map[string][]string{}
		for _, p := range a2GetPermissionsForUser(user()) {
			out[dashDeref(p.StationId)] = append(out[dashDeref(p.StationId)], dashDeref(p.Permission))
		}
		for k := range out {
			sort.Strings(out[k])
		}
		return out
	}

	w := do(t, s, "GET", "/api/users/"+uid+"/global_permissions", "", adminHdr)
	if st := read(w.Code, w.Body.Bytes()); len(st.Permissions) != 0 || len(st.Known) < 30 {
		t.Fatalf("initial: %s", w.Body.String())
	}

	// set: trimmed, lower-cased, de-duplicated; applied as a role on station "global"
	w = do(t, s, "PUT", "/api/users/"+uid+"/global_permissions", `{"permissions":[" user_kick ","USER_MUTE","user_kick","level_editor",""]}`, adminHdr)
	if st := read(w.Code, w.Body.Bytes()); strings.Join(st.Permissions, ",") != "user_kick,user_mute,level_editor" {
		t.Fatalf("after put: %s", w.Body.String())
	}
	if e := effective(); strings.Join(e["global"], ",") != "level_editor,user_kick,user_mute" {
		t.Fatalf("effective permissions: %v", e)
	}
	if r := user().Roles; len(r) != 2 || *r[0] != "keep-me" || *r[1] != dashGlobalPermsRoleId(uid) {
		t.Fatalf("user roles: %v", r)
	}
	// the search shows the count
	if w := do(t, s, "GET", "/api/users/search", "", adminHdr); !strings.Contains(w.Body.String(), `"global_permissions":3`) {
		t.Fatalf("search count: %s", w.Body.String())
	}

	// replace with a different set: one role, updated in place
	do(t, s, "PUT", "/api/users/"+uid+"/global_permissions", `{"permissions":["fleet:join"]}`, adminHdr)
	if e := effective(); strings.Join(e["global"], ",") != "fleet:join" {
		t.Fatalf("after replace: %v", e)
	}
	if n := docdb.GetCollection(Database, true, models.NewRoleResponse).Count(func(r *models.RoleResponse) bool {
		return strings.HasPrefix(dashDeref(r.RoleId), dashGlobalPermsRolePrefix)
	}); n != 1 {
		t.Fatalf("expected exactly one per-user global role, got %d", n)
	}

	// invalid input
	for _, bad := range []string{`{"permissions":["has space"]}`, `{"permissions":"x"}`, `{}`, `nope`} {
		if w := do(t, s, "PUT", "/api/users/"+uid+"/global_permissions", bad, adminHdr); w.Code != 400 {
			t.Fatalf("PUT %s: want 400, got %d", bad, w.Code)
		}
	}
	if w := do(t, s, "GET", "/api/users/nope/global_permissions", "", adminHdr); w.Code != 404 {
		t.Fatalf("unknown user: %d", w.Code)
	}

	// empty list removes the role and keeps the user's other roles
	do(t, s, "PUT", "/api/users/"+uid+"/global_permissions", `{"permissions":[]}`, adminHdr)
	if r := user().Roles; len(r) != 1 || *r[0] != "keep-me" {
		t.Fatalf("roles after clearing: %v", r)
	}
	if dashGlobalPermsRole(uid) != nil {
		t.Fatal("role must be deleted when cleared")
	}

	// master key only
	all := []string{}
	for _, p := range dashScopedPerms {
		all = append(all, `"`+p.Name+`"`)
	}
	w = do(t, s, "POST", "/api/scoped-tokens", `{"name":"everything","permissions":[`+strings.Join(all, ",")+`],"stations":["*"]}`, adminHdr)
	var tok map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &tok)
	th := map[string]string{"x-dashboard-key": tok["token"].(string)}
	for _, m := range []string{"GET", "PUT"} {
		if w := do(t, s, m, "/api/users/"+uid+"/global_permissions", `{"permissions":["admin"]}`, th); w.Code != 403 {
			t.Fatalf("scoped %s must be refused: %d", m, w.Code)
		}
	}
}
