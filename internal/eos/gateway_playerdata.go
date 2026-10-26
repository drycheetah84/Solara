// Port of AUnrealFeatures.EOSSDK/EosGatewayServer.PlayerDataStorage.cs.
//
// EOS Player Data Storage. Real traffic observed:
//
//	GET /datastorage/v2/access/deployment/{deploymentId}/private/{productUserId}/{filename}
//	    ?getDuration=300&putDuration=300&full=true
//
// which returns short-lived "presigned" read/write URLs the SDK then hits
// directly to transfer file bytes. Locally the "presigned" URLs point back at
// this gateway's /data/ endpoints with no real signing.
// Files are stored flat on disk under playerdata/{deploymentId}/{productUserId}/{filename}.
package eos

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/stj"
)

func playerDataRoot() string { return pathCombine(BaseDirectory, "playerdata") }

// unsafePlayerDataComponent rejects route values that could leave the
// playerdata/ tree (security review finding F1; defence in depth on top of
// the hosting layer's dot-segment removal).
func unsafePlayerDataComponent(s string) bool {
	if s == "" || s == "." || s == ".." || strings.ContainsAny(s, "/\\:\x00") || filepath.IsAbs(s) || filepath.VolumeName(s) != "" {
		return true
	}
	u, err := url.PathUnescape(s)
	return err != nil || u == "." || u == ".." || strings.ContainsAny(u, "/\\:\x00")
}

func playerDataFilePath(deploymentId, productUserId, filename string) string {
	for _, comp := range []string{deploymentId, productUserId, filename} {
		if unsafePlayerDataComponent(comp) {
			dotnet.Throw("System.UnauthorizedAccessException", "Access to the path is denied.")
		}
	}
	root, _ := filepath.Abs(playerDataRoot())
	final, _ := filepath.Abs(pathCombine(root, deploymentId, productUserId, pathGetFileName(filename)))
	if rel, err := filepath.Rel(root, final); err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		dotnet.Throw("System.UnauthorizedAccessException", "Access to the path is denied.")
	}
	dir := pathCombine(playerDataRoot(), deploymentId, productUserId)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		dotnet.Throw("System.IO.IOException", err.Error())
	}
	// filenames are opaque EOS-assigned strings, but guard against path traversal regardless.
	safeName := pathGetFileName(filename)
	return pathCombine(dir, safeName)
}

// PlayerDataStorageAccess mirrors EosGatewayServer.PlayerDataStorageAccess.
func (s *EosGatewayServer) PlayerDataStorageAccess(c *hosting.Context) hosting.Result {
	deploymentID, productUserID, filename := c.Param("deployment_id"), c.Param("product_user_id"), c.Param("filename")
	path := playerDataFilePath(deploymentID, productUserID, filename)
	exists := fileExists(path)

	baseUrl := fmt.Sprintf("http://%s:%d/datastorage/v2/data/deployment/%s/private/%s/%s",
		s.HTTP.Hostname, s.HTTP.Port, deploymentID, productUserID, filename)

	var fileMeta any
	if exists {
		info, err := os.Stat(path)
		if err != nil {
			dotnet.Throw("System.IO.IOException", err.Error())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			dotnet.Throw("System.IO.IOException", err.Error())
		}
		sum := md5.Sum(data)
		fileMeta = stj.O(
			"filename", filename,
			"md5Hash", hex.EncodeToString(sum[:]),
			"lastModified", dotnet.FromTime(info.ModTime().UTC(), dotnet.Utc).Format("yyyy-MM-ddTHH:mm:ss.fffZ"),
		)
	}

	result := stj.O(
		"accountId", productUserID,
		"deploymentId", deploymentID,
		"writeRequests", []stj.Obj{stj.O("id", dotnet.NewGuidN(), "uri", baseUrl)},
	)
	if exists {
		result.Set("readRequests", []stj.Obj{stj.O("id", dotnet.NewGuidN(), "deviceId", nil, "uri", baseUrl)})
		result.Set("file", fileMeta)
	} else {
		result.Set("readRequests", []any{})
	}

	return hosting.Configurable(200, "application/json", stj.MustMarshal(result))
}

// PlayerDataStorageGet mirrors EosGatewayServer.PlayerDataStorageGet.
func (s *EosGatewayServer) PlayerDataStorageGet(c *hosting.Context) hosting.Result {
	path := playerDataFilePath(c.Param("deployment_id"), c.Param("product_user_id"), c.Param("filename"))
	if !fileExists(path) {
		return hosting.NotFound(nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		dotnet.Throw("System.IO.IOException", err.Error())
	}
	return hosting.Configurable(200, "application/octet-stream", data)
}

// PlayerDataStoragePut mirrors EosGatewayServer.PlayerDataStoragePut.
func (s *EosGatewayServer) PlayerDataStoragePut(c *hosting.Context) hosting.Result {
	path := playerDataFilePath(c.Param("deployment_id"), c.Param("product_user_id"), c.Param("filename"))
	if err := os.WriteFile(path, c.Request.Body(), 0o644); err != nil {
		dotnet.Throw("System.IO.IOException", err.Error())
	}
	return hosting.Ok(nil)
}
