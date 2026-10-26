// Ports Http/Preprocessors/Default/HttpStorageContainerProcessor.cs and
// AstraHttpServerExtensions.AddStorageContainers.
//
// No original server registers this preprocessor (verified: no call to
// AddStorageContainers in the source); it is ported for framework parity.
// Original behaviour preserved verbatim, including its defects (see
// KNOWN_ISSUES.md HOST-SC-*):
//   - request.JsonBody values are JsonElement, so every `x is string` test is
//     false: file upload/delete always answer 400 "Container name is required."
//   - `(string)jsonElement` casts throw InvalidCastException (-> 500 page) in
//     create (when "name" is present) and delete.
//   - the download branch requires Uri.Split('/') to have exactly 2 parts,
//     impossible for a path starting with '/', so downloads never match.
package hosting

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

// ContainerInformation mirrors the C# class of the same name.
type ContainerInformation struct {
	Name        *string
	Description *string
	Owner       int
	CreatedAt   int64
	UpdatedAt   int64
	Readable    bool
	Writeable   bool
	Size        int64
	FileCount   int
}

// ContainerInformationSensitive mirrors the derived C# class (own members
// serialize before inherited ones, as with .NET reflection).
type ContainerInformationSensitive struct {
	Temporary bool
	ExpiresAt int64
	Ticket    *string
	ContainerInformation
}

// ContainerMessageResult mirrors the C# class.
type ContainerMessageResult struct {
	Success bool
	Message *string
}

// ContainerMessageResultOf mirrors ContainerMessageResult<T>.
type ContainerMessageResultOf[T any] struct {
	Value T
	ContainerMessageResult
}

func msg(ok bool, m string) ContainerMessageResult {
	return ContainerMessageResult{Success: ok, Message: &m}
}

// StorageContainerProcessor mirrors HttpStorageContainerProcessor.
type StorageContainerProcessor struct {
	server     *Server
	baseFolder string
	mu         sync.Mutex
	cache      map[string]*ContainerInformationSensitive
	sizes      map[string]int64
	counts     map[string]int
}

// AddStorageContainers mirrors AstraHttpServerExtensions.AddStorageContainers.
func (s *Server) AddStorageContainers(baseFolder string, containers []string) {
	for _, p := range s.preprocessors {
		if _, ok := p.(*StorageContainerProcessor); ok {
			return
		}
	}
	p := &StorageContainerProcessor{server: s, baseFolder: baseFolder,
		cache: map[string]*ContainerInformationSensitive{}, sizes: map[string]int64{}, counts: map[string]int{}}
	s.AddEndpoint("POST", "/api/v1/container/create", "HandleContainerCreate", p.create)
	s.AddEndpoint("POST", "/api/v1/container/delete", "HandleContainerDelete", p.delete)
	s.AddEndpoint("GET", "/api/v1/container/info", "HandleContainerInfo", p.info)
	s.AddEndpoint("POST", "/api/v1/container/file/upload", "HandleFileUpload", p.fileUpload)
	s.AddEndpoint("POST", "/api/v1/container/file/delete", "HandleFileDelete", p.fileDelete)
	s.AddEndpoint("GET", "/api/v1/container/list", "HandleContainerList", p.list)
	for _, c := range containers {
		_ = os.MkdirAll(filepath.Join(baseFolder, c), 0o755)
		p.loadMetadata(c)
	}
	s.preprocessors = append(s.preprocessors, p)
}

func (p *StorageContainerProcessor) loadMetadata(name string) {
	path := filepath.Join(p.baseFolder, name, "metadata.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var info ContainerInformationSensitive
	stj.MustUnmarshal(raw, &info)
	p.mu.Lock()
	p.cache[name] = &info
	p.mu.Unlock()
	p.updateStats(name)
}

func (p *StorageContainerProcessor) updateStats(name string) {
	var total int64
	count := 0
	_ = filepath.WalkDir(filepath.Join(p.baseFolder, name), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				total += fi.Size()
			}
			count++
		}
		return nil
	})
	p.mu.Lock()
	p.sizes[name], p.counts[name] = total, count
	info, ok := p.cache[name]
	if ok {
		info.Size, info.FileCount, info.UpdatedAt = total, count, dotnet.UtcNow().ToUnixSeconds()
	}
	p.mu.Unlock()
	if ok {
		p.saveMetadata(name, info)
	}
}

func (p *StorageContainerProcessor) saveMetadata(name string, info *ContainerInformationSensitive) {
	_ = os.WriteFile(filepath.Join(p.baseFolder, name, "metadata.json"), stj.MustMarshal(info), 0o644)
}

// TryPreprocessRequest mirrors the original (download branch unreachable for
// rooted paths; kept as written).
func (p *StorageContainerProcessor) TryPreprocessRequest(req *Request, resp *Response) (Result, PreprocessorResult) {
	parts := strings.Split(req.Uri(), "/")
	p.mu.Lock()
	_, known := p.cache[parts[0]]
	p.mu.Unlock()
	if req.Method() == "GET" && len(parts) == 2 && known {
		return p.download(parts[0], parts[1])
	}
	return NotFound(nil), PreFail
}

func castString(el stj.Element) string {
	// `(string)jsonElement` / `nameObj as string ?? throw new InvalidCastException()`
	dotnet.Throw("System.InvalidCastException", "Unable to cast object of type 'System.Text.Json.JsonElement' to type 'System.String'.")
	return ""
}

func (p *StorageContainerProcessor) create(c *Context) Result {
	body := c.Request.JsonBody()
	// GetValueOrDefault returns a boxed JsonElement (even for JSON null); the cast throws.
	nameEl, has := body.TryGetProperty("name")
	if has {
		castString(nameEl) // InvalidCastException, as `(string?)JsonElement` in the original
	}
	return BadRequest(msg(false, "Name is required."))
}

func (p *StorageContainerProcessor) delete(c *Context) Result {
	body := c.Request.JsonBody()
	nameEl, ok := body.TryGetProperty("name")
	if !ok {
		return BadRequest(msg(false, "Name is required."))
	}
	castString(nameEl)
	return nil
}

func (p *StorageContainerProcessor) info(c *Context) Result {
	name := c.Request.GetQueryParameter("name", "")
	if name == "" {
		return BadRequest(msg(false, "Name is required."))
	}
	p.mu.Lock()
	ci, ok := p.cache[name]
	p.mu.Unlock()
	if !ok {
		return NotFound(msg(false, "Container not found."))
	}
	m := "Container info retrieved successfully."
	return Ok(ContainerMessageResultOf[ContainerInformation]{Value: ci.ContainerInformation,
		ContainerMessageResult: ContainerMessageResult{Success: true, Message: &m}})
}

// fileUpload / fileDelete: the `nameObj is string` pattern never matches a
// JsonElement, so the first check always fails.
func (p *StorageContainerProcessor) fileUpload(c *Context) Result {
	_ = c.Request.JsonBody()
	return BadRequest(msg(false, "Container name is required."))
}

func (p *StorageContainerProcessor) fileDelete(c *Context) Result {
	_ = c.Request.JsonBody()
	return BadRequest(msg(false, "Container name is required."))
}

func (p *StorageContainerProcessor) download(container, file string) (Result, PreprocessorResult) {
	p.mu.Lock()
	ci, ok := p.cache[container]
	p.mu.Unlock()
	if !ok {
		return NotFound(msg(false, "Container not found.")), PreOK | PreStopAfter
	}
	if !ci.Readable {
		return Forbidden(msg(false, "Container is not readable.")), PreOK | PreStopAfter
	}
	path := filepath.Join(p.baseFolder, container, file)
	raw, err := os.ReadFile(path)
	if err != nil {
		return Forbidden(msg(false, "File not found.")), PreOK | PreStopAfter
	}
	return Configurable(200, MimeType(filepath.Base(path)), raw), PreOK | PreStopAfter
}

func (p *StorageContainerProcessor) list(c *Context) Result {
	var ownerFilter *int
	if v, ok := c.Request.Queries()["owner"]; ok {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 32); err == nil {
			x := int(n)
			ownerFilter = &x
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := []stj.Obj{}
	for _, ci := range p.cache {
		if ownerFilter != nil && ci.Owner != *ownerFilter {
			continue
		}
		var exp *int64
		if ci.ExpiresAt > 0 {
			e := ci.ExpiresAt
			exp = &e
		}
		out = append(out, stj.O("Name", ci.Name, "Description", ci.Description, "Owner", ci.Owner,
			"CreatedAt", ci.CreatedAt, "UpdatedAt", ci.UpdatedAt, "Size", ci.Size, "FileCount", ci.FileCount,
			"Readable", ci.Readable, "Writeable", ci.Writeable, "Temporary", ci.Temporary, "ExpiresAt", exp))
	}
	return Ok(out)
}
