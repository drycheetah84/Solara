package mothership

import (
	"crypto/md5"
	"encoding/hex"
	"testing"
)

// The embedded bodies must be the exact CDN bytes: S3/R2 ETags are the MD5 of the body.
func TestStaticObjectsMatchETag(t *testing.T) {
	for path, obj := range staticObjects {
		b, err := staticFS.ReadFile(obj.file)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		sum := md5.Sum(b)
		if want := `"` + hex.EncodeToString(sum[:]) + `"`; want != obj.etag {
			t.Errorf("%s: md5 %s != etag %s", path, want, obj.etag)
		}
	}
	idx, _ := staticFS.ReadFile("static/index.json")
	if len(idx) != 288 {
		t.Errorf("index.json is %d bytes, want 288", len(idx))
	}
}
