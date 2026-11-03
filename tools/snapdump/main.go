// snapdump: read-only dump of one station row from a Solara database file (use it on a COPY of a snapshot).
//
//	SOLARA_DB_PASSWORD=... snapdump <db file> <station id>
//
// Prints the station's name and its whole config map as JSON.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"solara/internal/ares/models"
	"solara/internal/docdb"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: snapdump <db file> <station id>")
		os.Exit(2)
	}
	var pw *string
	if v := os.Getenv("SOLARA_DB_PASSWORD"); v != "" {
		pw = &v
	}
	db, err := docdb.Open(&docdb.Descriptor{DatabaseId: 1, Name: "snapshot", Path: os.Args[1], Password: pw})
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer db.Close()
	col := docdb.GetCollection(db, true, models.NewStationDbObject)
	for _, s := range col.FindAll() {
		if s.StationId == nil || *s.StationId != os.Args[2] {
			continue
		}
		name := ""
		if s.StationName != nil {
			name = *s.StationName
		}
		out, _ := json.MarshalIndent(map[string]any{"station_id": *s.StationId, "station_name": name, "config": s.Config}, "", "  ")
		fmt.Println(string(out))
		return
	}
	fmt.Fprintln(os.Stderr, "station not found")
	os.Exit(1)
}
