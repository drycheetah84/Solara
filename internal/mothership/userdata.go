// Ports the user-data section of AUnrealFeatures.AAMothership/MothershipServer.cs:
// GetUserDataClient, the auto-completed-quests machinery (s_autoQuests,
// AutoCompleteQuests, DecodeQuestBlob, EncodeQuestBlob, ApplyAutoCompletedQuests),
// PostUserDataClient, DeleteUserData, CreateUserDataMetadata and
// ListUserDataMetadata.
package mothership

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

func userData() *docdb.Collection[models.MothershipUserDataDbObject] {
	return col(models.NewMothershipUserDataDbObject)
}

func findUserData(cl *docdb.Collection[models.MothershipUserDataDbObject], userId, keyName string) *models.MothershipUserDataDbObject {
	return cl.FindOne(func(d *models.MothershipUserDataDbObject) bool {
		return dbEq(d.UserId, userId) && dbEq(d.KeyName, keyName)
	})
}

// GetUserDataClient is the client-scoped user data GET (user_id from the JWT).
func (s *MothershipServer) GetUserDataClient(c *hosting.Context) hosting.Result {
	request := c.Request
	if !isClient(request) {
		authLog(fmt.Sprintf("[USERDATA] GET %s -> 401 (no valid x-mothership-token)", request.GetQueryParameter("key_name", "?")))
		return jsonError(401, "Unauthorized", "Missing or invalid session token", "", nil)
	}

	userId := getUserIdFromToken(request)
	keyName := request.GetQueryParameter("key_name", "")
	if userId == "" || keyName == "" {
		return jsonError(400, "BadRequest", "Missing key_name or token subject", "", &keyName)
	}

	cl := userData()
	item := findUserData(cl, userId, keyName)
	if keyName == "player_quests" {
		item = applyAutoCompletedQuests(cl, userId, item)
	}

	// A first-time key is not an error, it is an empty value (generation 0),
	// so the client can bootstrap (a 404 deadlocked inventory saves).
	if item == nil {
		authLog(fmt.Sprintf("[USERDATA] %s GET %s -> not stored yet, returning empty (bootstrap)", userId, keyName))
		empty := models.NewMothershipUserDataDbObject()
		empty.DataId, empty.UserId, empty.KeyName, empty.Value, empty.Generation = strp(""), strp(userId), strp(keyName), strp(""), 0
		return jsonT(empty, 200)
	}
	return jsonT(item, 200)
}

// ---- AUTO-COMPLETED QUESTS --------------------------------------------------
// Quests listed in auto_complete_quests.txt (next to the backend; one
// "<32-hex quest id> [completedVersion]" per line, '#' comments) are served as
// COMPLETED in every player's player_quests. player_quests blob: base64 of the
// UTF-8 JSON with every byte stored minus 1.

type autoQuest struct {
	Id  string
	Ver int
}

var (
	autoQuestsMu    sync.Mutex
	autoQuestsStamp time.Time // DateTime.MinValue
	autoQuestsList  = []autoQuest{}
)

// readAllLines mirrors File.ReadAllLines (BOM stripped; \r\n, \n, \r breaks).
func readAllLines(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := strings.TrimPrefix(utf8String(raw), "\xef\xbb\xbf")
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\r':
			lines = append(lines, s[start:i])
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			start = i + 1
		case '\n':
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines, nil
}

// autoCompleteQuests mirrors AutoCompleteQuests (reloaded when the file's
// last-write time changes; any failure yields an empty list).
func autoCompleteQuests() (out []autoQuest) {
	defer func() {
		if recover() != nil {
			out = []autoQuest{}
		}
	}()
	path := filepath.Join(baseDirectory(), "auto_complete_quests.txt")
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return []autoQuest{}
	}
	stamp := st.ModTime().UTC()
	autoQuestsMu.Lock()
	if stamp.Equal(autoQuestsStamp) {
		l := autoQuestsList
		autoQuestsMu.Unlock()
		return l
	}
	autoQuestsMu.Unlock()
	lines, err := readAllLines(path)
	if err != nil {
		return []autoQuest{}
	}
	list := []autoQuest{}
	for _, raw := range lines {
		line := strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		id := strings.ToUpper(strings.ReplaceAll(parts[0], "-", ""))
		if utf16Len(id) != 32 {
			continue
		}
		ver := 1
		if len(parts) > 1 {
			if v, err := strconv.ParseInt(parts[1], 10, 32); err == nil {
				ver = int(v)
			}
		}
		list = append(list, autoQuest{id, ver})
	}
	autoQuestsMu.Lock()
	autoQuestsStamp, autoQuestsList = stamp, list
	autoQuestsMu.Unlock()
	authLog(fmt.Sprintf("[QUESTS] auto-complete list loaded: %d quest(s) from %s", len(list), path))
	return list
}

// decodeQuestBlob mirrors DecodeQuestBlob (base64, every byte + 1, UTF-8).
func decodeQuestBlob(b64 string) string {
	b := fromBase64String(b64)
	for i := range b {
		b[i]++
	}
	return utf8String(b)
}

// encodeQuestBlob mirrors EncodeQuestBlob (UTF-8, every byte - 1, base64).
func encodeQuestBlob(json string) string {
	b := []byte(json)
	for i := range b {
		b[i]--
	}
	return base64.StdEncoding.EncodeToString(b)
}

// applyAutoCompletedQuests mirrors ApplyAutoCompletedQuests: marks every
// auto-complete quest as completed (progress 255) in the player's
// player_quests blob and writes the change back. Never panics.
func applyAutoCompletedQuests(cl *docdb.Collection[models.MothershipUserDataDbObject], userId string, item *models.MothershipUserDataDbObject) (ret *models.MothershipUserDataDbObject) {
	autos := autoCompleteQuests()
	if len(autos) == 0 {
		return item
	}
	ret = item
	defer func() {
		if p := recover(); p != nil {
			authLog(fmt.Sprintf("[QUESTS] auto-complete skipped for %s: %s", userId, hosting.ExceptionMessage(p)))
		}
	}()

	var root *jnode
	if item == nil || isNullOrEmpty(item.Value) {
		root = newObject()
		root.set("lastUpdate", strNode("0001.01.01-00.00.00"))
		root.set("quests", newArray())
		root.set("savedQuestProgressionVersion", intNode(3))
	} else {
		root = parseNode(decodeQuestBlob(*item.Value))
		if root == nil || root.kind != stj.Object {
			dotnet.Throw("System.IO.InvalidDataException", "player_quests is not a JSON object")
		}
	}

	quests := root.get("quests")
	if quests == nil || quests.kind != stj.Array {
		quests = newArray()
		root.set("quests", quests)
	}

	added, finished := 0, 0
	for _, aq := range autos {
		var q *jnode
		for _, x := range quests.items {
			if x == nil || x.kind != stj.Object {
				continue // OfType<JsonObject>()
			}
			if v := x.get("iD").asString(); v != nil && strings.EqualFold(*v, aq.Id) {
				q = x
				break
			}
		}
		if q == nil {
			nq := newObject()
			nq.set("iD", strNode(aq.Id))
			nq.set("progress", intNode(255))
			nq.set("completedVersion", intNode(aq.Ver))
			nq.set("completedTime", strNode("0001.01.01-00.00.00"))
			quests.items = append(quests.items, nq)
			added++
		} else if p := q.get("progress").asInt(); p == nil || *p != 255 {
			q.set("progress", intNode(255))
			q.set("completedVersion", intNode(aq.Ver))
			finished++
		}
	}
	if added == 0 && finished == 0 {
		return item
	}

	blob := encodeQuestBlob(root.toJsonStringIndented())
	if item == nil {
		item = models.NewMothershipUserDataDbObject()
		item.DataId, item.UserId, item.KeyName, item.Value, item.Generation = strp(newId()), strp(userId), strp("player_quests"), strp(blob), 1
		ret = item
		cl.Insert(item)
	} else {
		item.Value = strp(blob)
		item.Generation = item.Generation + 1
		cl.Update(item)
	}
	authLog(fmt.Sprintf("[QUESTS] auto-completed for %s: %d added, %d finished", userId, added, finished))
	return item
}

// PostUserDataClient is the client-scoped user data write (user_id from the JWT).
func (s *MothershipServer) PostUserDataClient(c *hosting.Context) hosting.Result {
	request := c.Request
	if !isClient(request) {
		authLog("[USERDATA] POST -> 401 (no valid x-mothership-token); this is a SAVE being dropped")
		return jsonError(401, "Unauthorized", "Missing or invalid session token", "", nil)
	}

	body := parseBody(request)
	userId := getUserIdFromToken(request)
	keyName := str(body, "key_name")
	if userId == "" || keyName == "" {
		return jsonError(400, "BadRequest", "Missing key_name or token subject", "", &keyName)
	}

	cl := userData()
	existing := findUserData(cl, userId, keyName)
	if existing != nil {
		existing.Value = strp(str(body, "value"))
		existing.Generation = existing.Generation + 1
		cl.Update(existing)
		authLog(fmt.Sprintf("[USERDATA] %s SAVED %s gen=%d bytes=%d", userId, keyName, existing.Generation, utf16Len(*existing.Value)))
		return jsonT(existing, 200)
	}

	data := models.NewMothershipUserDataDbObject()
	data.DataId = strp(newId())
	data.UserId = strp(userId)
	data.KeyName = strp(keyName)
	data.Value = strp(str(body, "value"))
	data.Generation = 1
	cl.Insert(data)
	authLog(fmt.Sprintf("[USERDATA] %s CREATED %s bytes=%d (first ever save of this key)", userId, keyName, utf16Len(*data.Value)))
	return jsonT(data, 200)
}

// DeleteUserData deletes a specific user data key for a player.
func (s *MothershipServer) DeleteUserData(c *hosting.Context) hosting.Result {
	if !isClient(c.Request) {
		return unauthorized()
	}
	userId := c.Request.GetQueryParameter("user_id", "")
	keyName := c.Request.GetQueryParameter("key_name", "")
	cl := userData()
	item := findUserData(cl, userId, keyName)
	if item == nil {
		return errorResult("User data not found", 404)
	}
	cl.Delete(item.Id)
	return jsonAnon(stj.O("status", "deleted"), 200)
}

// CreateUserDataMetadata creates a metadata definition for a user data key.
func (s *MothershipServer) CreateUserDataMetadata(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	metadata := models.NewMothershipUserDataMetadataDbObject()
	metadata.MetadataId = strp(newId())
	metadata.TitleId = strp(str(body, "title_id"))
	metadata.EnvId = strp(str(body, "env_id"))
	metadata.KeyName = strp(str(body, "key_name"))
	metadata.KeyPermissions = strp(str(body, "key_permissions"))
	metadata.PrivacyNotes = strp(str(body, "privacy_notes"))
	col(models.NewMothershipUserDataMetadataDbObject).Insert(metadata)
	return jsonT(metadata, 200)
}

// ListUserDataMetadata lists all user data metadata definitions.
func (s *MothershipServer) ListUserDataMetadata(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	results := col(models.NewMothershipUserDataMetadataDbObject).FindAll()
	return jsonAnon(stj.O("Results", results), 200)
}
