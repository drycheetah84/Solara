package docdb

import (
	"fmt"
	"reflect"

	bolt "go.etcd.io/bbolt"

	"solara/internal/dotnet"
)

// AstraDbObject mirrors the base class every stored model derives from.
// Embed it (by value) in a model struct. Like the C# property initializer
// `Id = ObjectId.NewObjectId()`, a zero Id is assigned on first Insert.
type AstraDbObject struct {
	Id dotnet.ObjectId `json:"-"`
}

func (o *AstraDbObject) GetId() dotnet.ObjectId   { return o.Id }
func (o *AstraDbObject) SetId(id dotnet.ObjectId) { o.Id = id }

// DbObject is implemented by *Model types embedding AstraDbObject.
type DbObject interface {
	GetId() dotnet.ObjectId
	SetId(dotnet.ObjectId)
}

// Collection mirrors ILiteCollection<T> for the operations the original used.
// Every read returns freshly decoded instances (LiteDB has no identity map).
type Collection[T any] struct {
	db   *Database
	name string
	newT func() *T
}

// Name returns the collection (bucket) name.
func (c *Collection[T]) Name() string { return c.name }

// LiteException mirrors LiteDB.LiteException.
type LiteException struct{ Msg string }

func (e *LiteException) Error() string { return e.Msg }

// GetCollection mirrors IDatabase.GetCollection<T>(createIfNotExisting).
// newT must construct T with its C# property initializers applied (the
// values LiteDB keeps for members absent from a stored document).
func GetCollection[T any](d *Database, createIfNotExisting bool, newT func() *T) *Collection[T] {
	if d == nil {
		return nil
	}
	t := reflect.TypeOf((*T)(nil)).Elem()
	if _, ok := any(new(T)).(DbObject); !ok {
		panic(fmt.Sprintf("docdb: %s does not embed AstraDbObject", t))
	}
	short, full := typeNames(t)
	name := CollectionName(short)
	if newT == nil {
		newT = func() *T { return new(T) }
	}
	col := &Collection[T]{db: d, name: name, newT: newT}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.meta.Has(name) {
		return col
	}
	if createIfNotExisting {
		_ = d.bolt.Update(func(tx *bolt.Tx) error {
			_, err := tx.CreateBucketIfNotExists([]byte(name))
			return err
		})
		d.meta.Add(name, full)
		d.meta.Save()
		return col
	}
	d.log.Warning("The database for type '{Type}' does not exist.", short)
	return nil
}

func (c *Collection[T]) decode(raw []byte) (*T, error) {
	plain, err := c.db.open(raw)
	if err != nil {
		return nil, err
	}
	v := c.newT()
	if err := decodeDoc(plain, reflect.ValueOf(v).Elem()); err != nil {
		return nil, fmt.Errorf("%s: %w", c.name, err)
	}
	return v, nil
}

func (c *Collection[T]) encode(v *T) ([]byte, error) {
	raw, err := encodeDoc(reflect.ValueOf(v).Elem())
	if err != nil {
		return nil, err
	}
	return c.db.seal(raw), nil
}

func idOf[T any](v *T) DbObject { return any(v).(DbObject) }

// scan iterates documents in _id order until fn returns false.
func (c *Collection[T]) scan(fn func(*T) bool) {
	err := c.db.bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(c.name))
		if b == nil {
			return nil
		}
		cur := b.Cursor()
		for k, v := cur.First(); k != nil; k, v = cur.Next() {
			doc, err := c.decode(v)
			if err != nil {
				return err
			}
			if !fn(doc) {
				return nil
			}
		}
		return nil
	})
	if err != nil {
		panic(&LiteException{err.Error()})
	}
}

// FindAll mirrors FindAll() (ascending _id).
func (c *Collection[T]) FindAll() []*T {
	out := []*T{}
	c.scan(func(t *T) bool { out = append(out, t); return true })
	return out
}

// Find mirrors Find(predicate). Use EqFold & co. for string comparisons to
// keep LiteDB's case-insensitive collation.
func (c *Collection[T]) Find(pred func(*T) bool) []*T {
	out := []*T{}
	c.scan(func(t *T) bool {
		if pred(t) {
			out = append(out, t)
		}
		return true
	})
	return out
}

// FindOne mirrors FindOne(predicate): first match in _id order, or nil.
func (c *Collection[T]) FindOne(pred func(*T) bool) *T {
	var hit *T
	c.scan(func(t *T) bool {
		if pred(t) {
			hit = t
			return false
		}
		return true
	})
	return hit
}

// FindById mirrors FindById(id).
func (c *Collection[T]) FindById(id dotnet.ObjectId) *T {
	var out *T
	err := c.db.bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(c.name))
		if b == nil {
			return nil
		}
		if raw := b.Get(id[:]); raw != nil {
			var err error
			out, err = c.decode(raw)
			return err
		}
		return nil
	})
	if err != nil {
		panic(&LiteException{err.Error()})
	}
	return out
}

// Count mirrors Count() / Count(predicate) (pred may be nil).
func (c *Collection[T]) Count(pred func(*T) bool) int {
	n := 0
	c.scan(func(t *T) bool {
		if pred == nil || pred(t) {
			n++
		}
		return true
	})
	return n
}

// Exists mirrors Exists(predicate).
func (c *Collection[T]) Exists(pred func(*T) bool) bool { return c.FindOne(pred) != nil }

func (c *Collection[T]) put(v *T, mustNotExist, mustExist bool) (existed bool) {
	obj := idOf(v)
	if obj.GetId().IsZero() {
		obj.SetId(dotnet.NewObjectId())
	}
	id := obj.GetId()
	raw, err := c.encode(v)
	if err != nil {
		panic(&LiteException{err.Error()})
	}
	err = c.db.bolt.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(c.name))
		if err != nil {
			return err
		}
		existed = b.Get(id[:]) != nil
		if mustNotExist && existed {
			return &LiteException{fmt.Sprintf(`Cannot insert duplicate key in unique index '_id'. The duplicate value is '{"$oid":"%s"}'.`, id.Hex())}
		}
		if mustExist && !existed {
			return nil
		}
		return b.Put(id[:], raw)
	})
	if err != nil {
		if le, ok := err.(*LiteException); ok {
			panic(le)
		}
		panic(&LiteException{err.Error()})
	}
	return existed
}

// Insert mirrors Insert(doc); panics with LiteException on a duplicate _id.
func (c *Collection[T]) Insert(v *T) dotnet.ObjectId {
	c.put(v, true, false)
	return idOf(v).GetId()
}

// InsertBulk mirrors InsertBulk / Insert(IEnumerable).
func (c *Collection[T]) InsertBulk(vs []*T) int {
	for _, v := range vs {
		c.Insert(v)
	}
	return len(vs)
}

// Update mirrors Update(doc): false when no document has that _id.
func (c *Collection[T]) Update(v *T) bool {
	if idOf(v).GetId().IsZero() {
		return false
	}
	return c.put(v, false, true)
}

// Upsert mirrors Upsert(doc): true when the document was inserted.
func (c *Collection[T]) Upsert(v *T) bool { return !c.put(v, false, false) }

// Delete mirrors Delete(id).
func (c *Collection[T]) Delete(id dotnet.ObjectId) bool {
	found := false
	err := c.db.bolt.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(c.name))
		if b == nil {
			return nil
		}
		if b.Get(id[:]) != nil {
			found = true
			return b.Delete(id[:])
		}
		return nil
	})
	if err != nil {
		panic(&LiteException{err.Error()})
	}
	return found
}

// DeleteMany mirrors DeleteMany(predicate) and returns the count removed.
func (c *Collection[T]) DeleteMany(pred func(*T) bool) int {
	var ids []dotnet.ObjectId
	c.scan(func(t *T) bool {
		if pred(t) {
			ids = append(ids, idOf(t).GetId())
		}
		return true
	})
	n := 0
	for _, id := range ids {
		if c.Delete(id) {
			n++
		}
	}
	return n
}

// DeleteAll mirrors DeleteAll().
func (c *Collection[T]) DeleteAll() int { return c.DeleteMany(func(*T) bool { return true }) }

// PutRaw stores an already-encoded document (import tooling).
func (c *Collection[T]) PutRaw(id dotnet.ObjectId, doc []byte) error {
	// validate it decodes into T
	if _, err := c.decodePlain(doc); err != nil {
		return err
	}
	return c.db.bolt.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(c.name))
		if err != nil {
			return err
		}
		return b.Put(id[:], c.db.seal(doc))
	})
}

func (c *Collection[T]) decodePlain(doc []byte) (*T, error) {
	v := c.newT()
	return v, decodeDoc(doc, reflect.ValueOf(v).Elem())
}
