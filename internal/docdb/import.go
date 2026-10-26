package docdb

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	bolt "go.etcd.io/bbolt"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

// ImportReport summarises an import.
type ImportReport struct {
	Collections map[string]int
}

// ImportLiteDBExport loads the output of tools/litedb-export (one
// "<collection>.jsonl" file per LiteDB collection, LiteDB extended JSON, plus
// the original "collections.meta") into d. Documents are stored verbatim (the
// codec reads LiteDB's extended-JSON forms), keyed by their original _id, so
// ids and _id ordering are preserved. Collections that already contain data
// are refused unless overwrite is set; nothing is deleted otherwise.
func ImportLiteDBExport(d *Database, dir string, overwrite bool) (*ImportReport, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	// collection name -> CLR type from the original meta sidecar, when present
	types := map[string]string{}
	if raw, err := os.ReadFile(filepath.Join(dir, "collections.meta")); err == nil {
		var m Bytedata
		if err := m.decode(raw); err == nil {
			for _, c := range m.Collections {
				types[c.CollectionName] = c.CollectionType
			}
		}
	}
	rep := &ImportReport{Collections: map[string]int{}}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		n, err := d.importFile(name, f, overwrite)
		if err != nil {
			return rep, fmt.Errorf("%s: %w", name, err)
		}
		rep.Collections[name] = n
		if !d.meta.Has(name) {
			typ := types[name]
			if typ == "" {
				typ = name
			}
			d.meta.Add(name, typ)
		}
	}
	d.meta.Save()
	return rep, nil
}

func (d *Database) importFile(name, path string, overwrite bool) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	type rec struct {
		id  dotnet.ObjectId
		doc []byte
	}
	var recs []rec
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	line := 0
	for sc.Scan() {
		line++
		b := bytes.TrimSpace(sc.Bytes())
		if len(b) == 0 {
			continue
		}
		el, err := stj.Parse(b)
		if err != nil {
			return 0, fmt.Errorf("line %d: %w", line, err)
		}
		idEl, ok := el.Prop("_id"), el.ValueKind() == stj.Object
		if !ok || idEl.IsUndefined() {
			return 0, fmt.Errorf("line %d: document has no _id", line)
		}
		oid, ok := wrapped(idEl, "$oid")
		if !ok {
			return 0, fmt.Errorf("line %d: _id is not an ObjectId", line)
		}
		id, err := dotnet.ParseObjectId(oid.Str())
		if err != nil {
			return 0, fmt.Errorf("line %d: %w", line, err)
		}
		recs = append(recs, rec{id, append([]byte(nil), b...)})
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	err = d.bolt.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(name))
		if err != nil {
			return err
		}
		if k, _ := b.Cursor().First(); k != nil && !overwrite {
			return fmt.Errorf("collection already contains data (use --overwrite)")
		}
		for _, r := range recs {
			if err := b.Put(r.id[:], d.seal(r.doc)); err != nil {
				return err
			}
		}
		return nil
	})
	return len(recs), err
}
