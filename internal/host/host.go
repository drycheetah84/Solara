// Package host ports AUnrealFeatures.Ares.Program.Main together with
// AUnrealFeatures.Hosting.Application.HostApplication: database setup, the
// collection pre-registration, DI registration of the servers (constructed -
// and therefore started - in registration order when the provider is built),
// and the cross-module hooks Program.cs wires up.
package host

import (
	"fmt"
	"strings"

	"solara/internal/ares"
	aresmodels "solara/internal/ares/models"
	"solara/internal/di"
	"solara/internal/docdb"
	"solara/internal/eos"
	"solara/internal/halcyonsocket"
	"solara/internal/logging"
	"solara/internal/mothership"
	msmodels "solara/internal/mothership/models"
	"solara/internal/secrets"
	"solara/internal/serverid"
)

// DefaultConfiguration is the emulated build configuration (the installers
// published Release builds).
const DefaultConfiguration = "Release"

// Host mirrors HostApplication.
type Host struct {
	Name     string
	services *di.ServiceHost
	manager  *docdb.Manager
	Database *docdb.Database
	stoppers []func()
}

// registerFullNames records the original CLR type names so the .meta sidecar
// lists the same CollectionType strings as the original.
func registerFullNames() {
	const a, m = "AUnrealFeatures.Ares.Models.", "AUnrealFeatures.AAMothership.Models."
	docdb.SetFullName[aresmodels.UserDataResponse](a + "UserDataResponse")
	docdb.SetFullName[aresmodels.RoleResponse](a + "RoleResponse")
	docdb.SetFullName[ares.DrafterGrant]("AUnrealFeatures.Ares.Servers.DrafterGrant")
	docdb.SetFullName[aresmodels.StationDbObject](a + "StationDbObject")
	docdb.SetFullName[aresmodels.DeploymentDbObject](a + "DeploymentDbObject")
	docdb.SetFullName[aresmodels.ServerEventDbObject](a + "ServerEventDbObject")
	docdb.SetFullName[aresmodels.StationEventDbObject](a + "StationEventDbObject")
	docdb.SetFullName[aresmodels.EventSignupDbObject](a + "EventSignupDbObject")
	docdb.SetFullName[aresmodels.UserApiKeyDbObject](a + "UserApiKeyDbObject")
	docdb.SetFullName[msmodels.MothershipPlayerDbObject](m + "MothershipPlayerDbObject")
	docdb.SetFullName[msmodels.MothershipSessionDbObject](m + "MothershipSessionDbObject")
	docdb.SetFullName[msmodels.MothershipV2PlayerDbObject](m + "MothershipV2PlayerDbObject")
	docdb.SetFullName[msmodels.MothershipAssociationDbObject](m + "MothershipAssociationDbObject")
	docdb.SetFullName[msmodels.MothershipLinkDbObject](m + "MothershipLinkDbObject")
	docdb.SetFullName[msmodels.MothershipSharedGroupDbObject](m + "MothershipSharedGroupDbObject")
	docdb.SetFullName[msmodels.MothershipReportDbObject](m + "MothershipReportDbObject")
	docdb.SetFullName[msmodels.MothershipBanDbObject](m + "MothershipBanDbObject")
	docdb.SetFullName[msmodels.MothershipMuteDbObject](m + "MothershipMuteDbObject")
}

func dbName(config string) (string, int, error) {
	switch strings.ToLower(config) {
	case "release":
		return "production", 1, nil
	case "debug":
		return "development", 2, nil
	}
	return "", 0, fmt.Errorf("unknown configuration %q (want Release or Debug)", config)
}

func newManager() *docdb.Manager {
	return docdb.NewBuilder().
		WithDatabase("production", docdb.Options{DatabaseId: 1, FriendlyName: "Production Database", UsePassword: true, Password: secrets.Get("PRODUCTION_DB_PASSWORD")}).
		WithDatabase("development", docdb.Options{DatabaseId: 2, FriendlyName: "Development Database", UsePassword: false}).
		Build()
}

// OpenDatabase opens the database a configuration selects (import tooling).
func OpenDatabase(config string) (*docdb.Database, func(), error) {
	name, id, err := dbName(config)
	if err != nil {
		return nil, nil, err
	}
	if name == "production" && secrets.Get("PRODUCTION_DB_PASSWORD") == "" {
		return nil, nil, fmt.Errorf("PRODUCTION_DB_PASSWORD is not configured (see docs/CONFIGURATION.md)")
	}
	registerFullNames()
	m := newManager()
	db, err := m.GetDatabase(name, id)
	if err != nil || db == nil {
		return nil, nil, fmt.Errorf("open %s database: %v", name, err)
	}
	return db, m.Close, nil
}

// Build performs everything Program.Main did before RunAsync.
func Build(config string) (*Host, error) {
	ModuleInitialize()
	if missing := secrets.Missing(); len(missing) > 0 {
		return nil, fmt.Errorf("legacy secrets not configured: %s (run tools/scripts/extract-legacy-secrets.sh; see docs/CONFIGURATION.md)", strings.Join(missing, ", "))
	}
	name, id, err := dbName(config)
	if err != nil {
		return nil, err
	}
	registerFullNames()
	h := &Host{Name: "Astra", services: di.NewServiceHost("Astra")}

	var cfgErr error
	h.services.AddDependencies(func(b *di.Builder) {
		h.manager = newManager()
		// AddSingleton<AstraDatabaseManager, IDatabaseManager>(manager): the class has
		// constructor parameters, so the DI quirk leaves the instance null (DI-001).
		b.AddSingleton("AstraDatabaseManager", "IDatabaseManager", true, h.manager, nil)
		db, err := h.manager.GetDatabase(name, id)
		if err != nil || db == nil {
			cfgErr = fmt.Errorf("open %s database: %v", name, err)
			return
		}
		h.Database = db
		ares.Database = db
		b.AddSingleton("AstraDatabase", "IDatabase", true, db, nil)

		mothership.Database = db
		// Solara (SEC-SID): per-server identity tokens live in the same database.
		serverid.SetDatabase(db)
		PreRegisterCollections(db)
	})
	if cfgErr != nil {
		return nil, cfgErr
	}

	// .AddServer<I, T>() registrations, in Program.cs order.
	add := func(class, iface string, ctor func() (any, func(), error)) {
		h.services.AddDependencies(func(b *di.Builder) {
			b.AddSingleton(class, iface, false, nil, func(di.Provider) (any, error) {
				inst, stop, err := ctor()
				if err != nil {
					return nil, err
				}
				h.stoppers = append(h.stoppers, stop)
				return inst, nil
			})
		})
	}
	add("A2StationDbServer", "IA2StationDbServer", func() (any, func(), error) { // :78
		s, err := ares.NewA2StationDbServer()
		if err != nil {
			return nil, nil, err
		}
		return s, s.HTTP.Stop, nil
	})
	add("AresDashboardServer", "IAresDashboardServer", func() (any, func(), error) { // :8080
		s, err := ares.NewAresDashboardServer()
		if err != nil {
			return nil, nil, err
		}
		return s, s.HTTP.Stop, nil
	})
	add("MothershipServer", "IMothershipServer", func() (any, func(), error) { // :90
		s, err := mothership.NewMothershipServer()
		if err != nil {
			return nil, nil, err
		}
		return s, s.HTTP.Stop, nil
	})
	add("EosGatewayServer", "IEosGatewayServer", func() (any, func(), error) { // :50 (+WS)
		s, err := eos.NewEosGatewayServer()
		if err != nil {
			return nil, nil, err
		}
		return s, s.HTTP.Stop, nil
	})
	add("EosWsServer", "IEosWsServer", func() (any, func(), error) { // :80
		s, err := eos.NewEosWsServer()
		if err != nil {
			return nil, nil, err
		}
		return s, s.HTTP.Stop, nil
	})
	add("HalcyonSocketServer", "IHalcyonSocketServer", func() (any, func(), error) { // :9095 + :9100
		s, err := halcyonsocket.NewHalcyonSocketServer()
		if err != nil {
			return nil, nil, err
		}
		return s, s.Stop, nil
	})

	// ServerWatchdog.TakeDeploymentOffline hook (assigned before the servers are
	// constructed, as in Program.cs which assigns it before RunAsync).
	halcyonsocket.TakeDeploymentOffline = TakeDeploymentOffline
	// Solara (F5): the :9095 control routes accept the dashboard admin key.
	halcyonsocket.AdminKeyValidator = ares.DashboardAuthPreprocessorHasValidAdminKey

	// Hand the EOS gateway every station's browser whitelist before serving.
	ares.PushAll()
	return h, nil
}

// TakeDeploymentOffline mirrors the lambda Program.cs assigns to
// ServerWatchdog.TakeDeploymentOffline.
func TakeDeploymentOffline(deploymentId, reason string) {
	dropped := eos.RemoveSessionsByDeployment(deploymentId)
	depCol := docdb.GetCollection(ares.Database, true, aresmodels.NewDeploymentDbObject)
	var dep *aresmodels.DeploymentDbObject
	if depCol != nil {
		for _, d := range depCol.FindAll() { // LINQ FirstOrDefault: case-sensitive
			if d.DeploymentId != nil && *d.DeploymentId == deploymentId {
				dep = d
				break
			}
		}
	}
	if dep != nil {
		dep.Online = false
		dep.PlayerCount = 0
		depCol.Update(dep)
	}
	state := "not found"
	if dep != nil {
		state = "marked offline"
	}
	// Solara (SEC-SID): a deployment taken offline loses its server tokens
	// (a watchdog restart mints a new one).
	_, _ = serverid.RevokeDeployment(deploymentId, "taken offline: "+reason)
	logging.Console(fmt.Sprintf("[Watchdog] took deployment %s out of the browser (%s): %d session(s) dropped, db row %s", deploymentId, reason, dropped, state))
}

// Run mirrors RunAsync: builds the service provider, which constructs (and
// starts) every server in registration order. A failure aborts startup.
func (h *Host) Run() error {
	_, err := h.services.GetServiceProvider()
	if err == nil {
		// automatic point-in-time snapshots (docs/SNAPSHOTS.md); stopped by
		// Shutdown before the database closes
		h.stoppers = append(h.stoppers, StartSnapshotScheduler(h.Database))
	}
	return err
}

// Shutdown stops servers in reverse order and closes the database.
func (h *Host) Shutdown() {
	for i := len(h.stoppers) - 1; i >= 0; i-- {
		if h.stoppers[i] != nil {
			h.stoppers[i]()
		}
	}
	if h.manager != nil {
		h.manager.Close()
	}
}

// PreRegisterCollections mirrors the GetCollection<T>(true) calls Program.cs
// makes at startup, in the same order (creating collections + .meta entries).
func PreRegisterCollections(db *docdb.Database) {
	registerFullNames()
	// -- stations, deployments, events, dashboard accounts --
	docdb.GetCollection(db, true, aresmodels.NewUserDataResponse)
	docdb.GetCollection(db, true, aresmodels.NewRoleResponse)
	docdb.GetCollection(db, true, ares.NewDrafterGrant)
	docdb.GetCollection(db, true, aresmodels.NewStationDbObject)
	docdb.GetCollection(db, true, aresmodels.NewDeploymentDbObject)
	docdb.GetCollection(db, true, aresmodels.NewServerEventDbObject)
	docdb.GetCollection(db, true, aresmodels.NewStationEventDbObject)
	docdb.GetCollection(db, true, aresmodels.NewEventSignupDbObject)
	docdb.GetCollection(db, true, aresmodels.NewUserApiKeyDbObject)
	// -- Mothership --
	docdb.GetCollection(db, true, msmodels.NewMothershipPlayerDbObject)
	docdb.GetCollection(db, true, msmodels.NewMothershipSessionDbObject)
	docdb.GetCollection(db, true, msmodels.NewMothershipV2PlayerDbObject)
	docdb.GetCollection(db, true, msmodels.NewMothershipAssociationDbObject)
	docdb.GetCollection(db, true, msmodels.NewMothershipLinkDbObject)
	docdb.GetCollection(db, true, msmodels.NewMothershipSharedGroupDbObject)
	docdb.GetCollection(db, true, msmodels.NewMothershipReportDbObject)
	docdb.GetCollection(db, true, msmodels.NewMothershipBanDbObject)
	docdb.GetCollection(db, true, msmodels.NewMothershipMuteDbObject)
	// -- Solara additions (not in Program.cs) --
	docdb.GetCollection(db, true, serverid.NewServerTokenDbObject)
}
