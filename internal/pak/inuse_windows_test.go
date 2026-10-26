//go:build windows

package pak

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

// A handle opened without FILE_SHARE_DELETE (like the game's) must make
// Save abort with ErrFileInUse and leave everything untouched.
func TestSaveFileInUse(t *testing.T) {
	dir := t.TempDir()
	pak := writeTestPak(t, dir)
	orig := fileBytes(t, pak)
	h, err := os.Open(pak) // Go opens without FILE_SHARE_DELETE
	if err != nil {
		t.Fatal(err)
	}
	_, err = Save(pak, map[string][]byte{"A2/Config/DefaultGame.ini": []byte("[X]\r\n")}, nil)
	h.Close()
	if !errors.Is(err, ErrFileInUse) {
		t.Fatalf("expected ErrFileInUse, got %v", err)
	}
	if !bytes.Equal(fileBytes(t, pak), orig) {
		t.Fatal("pak modified")
	}
	if _, err := os.Stat(TempPath(pak)); err == nil {
		t.Fatal("temp left behind")
	}
}
