// Ports the static state of AUnrealFeatures.Ares.Program (Program.cs). The
// startup sequence itself lives in cmd/solara/main.go.
package ares

import "solara/internal/docdb"

// Database mirrors Program.Database: the active AstraDatabase ("development"
// in Debug builds, "production" in Release), set before any server starts.
var Database *docdb.Database
