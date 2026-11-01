// Ports the title / environment / deployment automation and title-data
// endpoints of AUnrealFeatures.AAMothership/MothershipServer.cs:
// AutomationCreateTitle, AutomationGetTitles, AutomationCreateEnvironment,
// AutomationGetEnvironments, AutomationCreateDeployment, AutomationGetDeployments,
// PostTitleData, GetTitleData, DeleteTitleData, ClientGetTitleData,
// AutomationSetTitleData, AutomationGetTitleData and AutomationDeleteTitleData.
package mothership

import (
	"sort"
	"time"

	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

// orNewId mirrors `Str(...) is { Length: > 0 } s ? s : NewId()`.
func orNewId(s string) string {
	if len(s) > 0 {
		return s
	}
	return newId()
}

func (s *MothershipServer) AutomationCreateTitle(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	title := models.NewMothershipTitleDbObject()
	title.TitleId = strp(orNewId(str(body, "title_id")))
	title.TitleName = strp(str(body, "title_name"))
	col(models.NewMothershipTitleDbObject).Insert(title)
	return jsonT(title, 200)
}

func (s *MothershipServer) AutomationGetTitles(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	titleId, _ := c.Request.Query("titleId")
	if titleId != "" {
		title := col(models.NewMothershipTitleDbObject).FindOne(func(t *models.MothershipTitleDbObject) bool {
			return dbEq(t.TitleId, titleId)
		})
		if title != nil {
			return jsonT(title, 200)
		}
	}
	results := col(models.NewMothershipTitleDbObject).FindAll()
	return jsonAnon(stj.O("Results", results), 200)
}

func (s *MothershipServer) AutomationCreateEnvironment(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	env := models.NewMothershipEnvironmentDbObject()
	env.EnvId = strp(orNewId(str(body, "envId")))
	env.TitleId = strp(str(body, "titleId"))
	env.EnvName = strp(str(body, "envName"))
	env.RequiredTags = strPtrs(strList(body, "requiredTags"))
	col(models.NewMothershipEnvironmentDbObject).Insert(env)
	return jsonAnon(stj.O("env", env), 200)
}

func (s *MothershipServer) AutomationGetEnvironments(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	envId, _ := c.Request.Query("envId")
	titleId, _ := c.Request.Query("titleId")
	cl := col(models.NewMothershipEnvironmentDbObject)

	if envId != "" {
		env := cl.FindOne(func(e *models.MothershipEnvironmentDbObject) bool { return dbEq(e.EnvId, envId) })
		if env != nil {
			return jsonT(env, 200)
		}
	} else if titleId != "" {
		results := cl.Find(func(e *models.MothershipEnvironmentDbObject) bool { return dbEq(e.TitleId, titleId) })
		return jsonAnon(stj.O("Results", results), 200)
	}
	return jsonAnon(stj.O("Results", cl.FindAll()), 200)
}

func (s *MothershipServer) AutomationCreateDeployment(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	deployment := models.NewMothershipDeploymentDbObject()
	deployment.DeploymentId = strp(orNewId(str(body, "deployment_id")))
	deployment.TitleId = strp(str(body, "title_id"))
	deployment.EnvId = strp(str(body, "env_id"))
	deployment.Name = strp(str(body, "name"))
	deployment.RequiredTags = strPtrs(strList(body, "requiredTags"))
	col(models.NewMothershipDeploymentDbObject).Insert(deployment)
	return jsonAnon(stj.O("deployment", deployment), 200)
}

func (s *MothershipServer) AutomationGetDeployments(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	deploymentId, _ := c.Request.Query("deployment_id")
	cl := col(models.NewMothershipDeploymentDbObject)
	if deploymentId != "" {
		dep := cl.FindOne(func(d *models.MothershipDeploymentDbObject) bool {
			return dbEq(d.DeploymentId, deploymentId)
		})
		if dep != nil {
			return jsonT(dep, 200)
		}
	}
	return jsonAnon(stj.O("Results", cl.FindAll()), 200)
}

// ---- TITLE DATA -------------------------------------------------------------

func titleData() *docdb.Collection[models.MothershipTitleDataDbObject] {
	return col(models.NewMothershipTitleDataDbObject)
}

func findTitleData(cl *docdb.Collection[models.MothershipTitleDataDbObject], key string) *models.MothershipTitleDataDbObject {
	return cl.FindOne(func(d *models.MothershipTitleDataDbObject) bool { return dbEq(d.Key, key) })
}

// setTitleData is the shared body of PostTitleData / AutomationSetTitleData
// (the original duplicated it verbatim).
func setTitleData(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	key := str(body, "key")
	cl := titleData()
	item := findTitleData(cl, key)
	if item != nil {
		item.Value = strp(str(body, "value"))
		item.UpdatedAt = strp(utcNowO())
		cl.Update(item)
		return jsonT(item, 200)
	}
	newItem := models.NewMothershipTitleDataDbObject()
	newItem.Key = strp(key)
	newItem.Value = strp(str(body, "value"))
	newItem.UpdatedAt = strp(utcNowO())
	cl.Insert(newItem)
	return jsonT(newItem, 200)
}

// deleteTitleData is the shared body of DeleteTitleData / AutomationDeleteTitleData.
func deleteTitleData(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	key := c.Request.GetQueryParameter("key", "")
	cl := titleData()
	item := findTitleData(cl, key)
	if item == nil {
		return errorResult("Title data not found", 404)
	}
	cl.Delete(item.Id)
	return jsonAnon(stj.O("status", "deleted"), 200)
}

// PostTitleData creates or updates a title data key-value pair.
func (s *MothershipServer) PostTitleData(c *hosting.Context) hosting.Result { return setTitleData(c) }

// GetTitleData returns title data, optionally filtered by key.
func (s *MothershipServer) GetTitleData(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	key, _ := c.Request.Query("key")
	cl := titleData()
	if key != "" {
		if item := findTitleData(cl, key); item != nil {
			return jsonT(item, 200)
		}
	}
	return jsonAnon(stj.O("Results", cl.FindAll()), 200)
}

// DeleteTitleData deletes a title data entry by key.
func (s *MothershipServer) DeleteTitleData(c *hosting.Context) hosting.Result {
	return deleteTitleData(c)
}

// TitleDataExtra (Solara) supplies extra client title-data entries: the live station's board image URLs. Set by
// the station package at startup (it cannot be imported from here). 50451 boards whose ImageLoader Source is
// MothershipTitleData (BoardTextureUrl1/2/11, ShuttlePosterUrl, ShuttleUrl4-6) read their URL from this response,
// not from the station config -- without this an uploaded board never showed.
var TitleDataExtra func() map[string]string

// ClientGetTitleData returns all title data (no auth required). Stored keys win over TitleDataExtra.
func (s *MothershipServer) ClientGetTitleData(c *hosting.Context) hosting.Result {
	items := titleData().FindAll()
	if TitleDataExtra != nil {
		have := map[string]bool{}
		for _, it := range items {
			have[sv(it.Key)] = true
		}
		extra := TitleDataExtra()
		keys := make([]string, 0, len(extra))
		for k := range extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if have[k] || extra[k] == "" {
				continue
			}
			it := models.NewMothershipTitleDataDbObject()
			it.Key, it.Value = strp(k), strp(extra[k])
			items = append(items, it)
		}
	}
	// 50451 client parser (0x4DCBFA0): Results is an ARRAY and every entry needs strings title_id, updated_at, key,
	// data and a bool server_only (env_id / deployment_id optional) -- an entry missing one fails the whole load,
	// so the stored {key, value} rows were never usable by this build. Emit that shape ("value" kept alongside
	// "data" for anything that read the old field).
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	out := make([]any, 0, len(items))
	for _, it := range items {
		upd := sv(it.UpdatedAt)
		if upd == "" {
			upd = now
		}
		out = append(out, stj.O("title_id", "a2", "updated_at", upd, "key", sv(it.Key), "data", sv(it.Value),
			"value", sv(it.Value), "server_only", false))
	}
	return jsonAnon(stj.O("Results", out), 200)
}

// AutomationSetTitleData creates or updates a title data entry.
func (s *MothershipServer) AutomationSetTitleData(c *hosting.Context) hosting.Result {
	return setTitleData(c)
}

// AutomationGetTitleData returns all title data.
func (s *MothershipServer) AutomationGetTitleData(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("Results", titleData().FindAll()), 200)
}

// AutomationDeleteTitleData deletes a title data entry by key.
func (s *MothershipServer) AutomationDeleteTitleData(c *hosting.Context) hosting.Result {
	return deleteTitleData(c)
}
