// [EREBUSDIR] v3 -- seven-zone objective chain for the EREBUS PROTOCOL live event (server only; included by dllmain_50451.cpp).
// Chain: relays -> grid -> valves -> cores -> resonators -> conduits -> shards. Each objective activates when the
// previous one is done; finishing one plays its zone-door sequence (EREBUS_SEQ_DOORS_<id>, replicated playback),
// lights its zone counter, and releases the matching show gate (EREBUS_GATE|<id>|t|timeout) so the replicated
// sequence resumes. Visual state = replicated actors hidden/shown; sounds = A2 Wwise events via the announcer.
namespace ebdir {
struct Gate { std::string id; double t = 0, timeout = 0; bool done = false, paused = false; ULONGLONG pausedAt = 0; };
struct Cue { double t = 0; std::string path; bool fired = false; };
static const char* kChain[] = { "relays", "grid", "valves", "cores", "resonators", "conduits", "shards" };
static constexpr int kN = 7;
static SDK::ALevelSequenceActor* s_show = nullptr;
static std::unordered_map<std::string, SDK::AActor*> s_act;
static std::vector<SDK::AActor*> s_beams;
static std::vector<Gate> s_gates;
static std::vector<Cue> s_cues;
static std::unordered_map<std::string, std::string> s_sfx;
static std::unordered_map<std::string, SDK::UObject*> s_loaded;
static double s_shardsAt = 1e9, s_conduitsAt = 1e9, s_charge = 0;
static bool s_done[kN] = {}, s_started[kN] = {};
static bool s_relayOn[4] = {}, s_valveOn[3] = {}, s_core[5] = {}, s_shard[30] = {}, s_resoLit[4] = {}, s_resoTouch[4] = {};
static double s_valveHold[3] = {};
static int s_resoOrder[4] = { 2, 0, 3, 1 }, s_resoStep = 0;
static int s_shardCount = 0, s_coreCount = 0, s_chargeShown = -1;
static std::unordered_map<void*, ULONGLONG> s_zapAt;
static bool s_skipReq = false;

static std::vector<std::string> Split(const std::string& s, char d)
{
    std::vector<std::string> out; size_t a = 0;
    for (;;) { size_t b = s.find(d, a); out.push_back(s.substr(a, b == std::string::npos ? std::string::npos : b - a)); if (b == std::string::npos) break; a = b + 1; }
    return out;
}
static SDK::AActor* A(const std::string& tag) { auto it = s_act.find(tag); return it == s_act.end() ? nullptr : it->second; }
static void Hide(const std::string& tag, bool h) { if (SDK::AActor* a = A(tag)) if (a->bHidden != h) a->SetActorHiddenInGame(h); }
static std::string Tg(const char* fmt, int i) { char t[96]; snprintf(t, sizeof t, fmt, i); return t; }
static double Dist(const SDK::FVector& a, const SDK::FVector& b) { const double x = a.X - b.X, y = a.Y - b.Y, z = a.Z - b.Z; return sqrt(x * x + y * y + z * z); }
static int Idx(const std::string& id) { for (int i = 0; i < kN; ++i) if (id == kChain[i]) return i; return -1; }

static SDK::UObject* Load(const std::string& path)
{
    auto it = s_loaded.find(path);
    if (it != s_loaded.end() && it->second) return it->second;
    const std::wstring w(path.begin(), path.end());
    SDK::FSoftObjectPath sp = SDK::UKismetSystemLibrary::MakeSoftObjectPath(SDK::FString(w.c_str()));
    SDK::UObject* o = SDK::UKismetSystemLibrary::LoadAsset_Blocking(SDK::UKismetSystemLibrary::Conv_SoftObjPathToSoftObjRef(sp));
    s_loaded[path] = o;
    if (!o) HxLog("[HalcyonA2][EREBUSDIR] could not load %s\n", path.c_str());
    return o;
}
static void Sound(const std::string& path)
{
    SDK::UObject* ev = Load(path);
    SDK::UObject* mgr = GaiaManager();
    if (!ev || !mgr) return;
    static_cast<SDK::UA2AnnouncementManagerComponent*>(mgr)->Broadcast_PlayAnnouncementEvent(static_cast<SDK::UAkAudioEvent*>(ev));
}
static void Sfx(const char* key) { auto it = s_sfx.find(key); if (it != s_sfx.end()) Sound(it->second); }
static void Counter(const char* base, int from, int to)
{
    char t[96];
    if (from >= 0) { snprintf(t, sizeof t, "%s_%d", base, from); Hide(t, true); }
    if (to >= 0)   { snprintf(t, sizeof t, "%s_%d", base, to);   Hide(t, false); }
}
static void CounterClear(const char* base, int n) { for (int k = 0; k <= n; ++k) Counter(base, k, -1); }
// State sequences (EREBUS_SEQ_<NAME>, built by build_erebus_stateseq.py): replicated, pause-at-end LevelSequences
// that MOVE geometry on every client. Replicated actor movement never reached clients (verified 2026-11-03: server door
// z -27800 -> -26750, client stayed at -27800); replicated sequence playback does, and carries collision with it.
static SDK::ULevelSequencePlayer* StatePlayer(const std::string& name)
{
    SDK::AActor* a = A("EREBUS_SEQ_" + name);
    if (!a || !a->IsA(SDK::ALevelSequenceActor::StaticClass())) return nullptr;
    return static_cast<SDK::ALevelSequenceActor*>(a)->SequencePlayer;
}
static bool PlayState(const std::string& name)
{
    SDK::ULevelSequencePlayer* p = StatePlayer(name);
    HxLog("[HalcyonA2][EREBUSDIR][STATE] play %s -> %s\n", name.c_str(), p ? "ok" : "MISSING");
    if (!p) return false;
    p->Play();
    return true;
}
static void RewindState(const std::string& name)
{
    SDK::ULevelSequencePlayer* p = StatePlayer(name);
    if (!p) return;
    SDK::FMovieSceneSequencePlaybackParams prm{};
    prm.PositionType = SDK::EMovieScenePositionType::Frame;
    prm.UpdateMethod = SDK::EUpdatePositionMethod::Jump;
    p->SetPlaybackPosition(prm);
    p->Pause();
}
static bool CurFrame(SDK::ULevelSequencePlayer* p, SDK::FFrameTime& out)
{
    static SDK::UFunction* fn = nullptr;          // windows.h #defines GetCurrentTime -> call the UFunction directly
    if (!fn && p && p->Class) fn = p->Class->GetFunction("MovieSceneSequencePlayer", "GetCurrentTime");
    if (!fn || !p) return false;
    struct { SDK::FQualifiedFrameTime ReturnValue; } tp{};
    if (!SafeProcessEvent(p, fn, &tp)) return false;
    out = tp.ReturnValue.Time;
    return true;
}
// Late join: a client that connects while a sequence is PAUSED (a door/bridge state at its end, or the show held at a
// gate) never evaluates it (verified 2026-11-03: late joiner saw the relays door shut). Re-jumping a paused player to
// its own position multicasts the position to every client, so the newcomer evaluates it too.
static void ResyncPaused(SDK::ULevelSequencePlayer* p, const char* what)
{
    SDK::FFrameTime ft{};
    if (!p || !p->IsPaused() || !CurFrame(p, ft)) return;
    SDK::FMovieSceneSequencePlaybackParams prm{};
    prm.Frame = ft;
    prm.PositionType = SDK::EMovieScenePositionType::Frame;
    prm.UpdateMethod = SDK::EUpdatePositionMethod::Jump;
    p->SetPlaybackPosition(prm);
    p->Pause();
    HxLog("[HalcyonA2][EREBUSDIR][STATE] resync %s @frame %d\n", what, ft.FrameNumber.Value);
}
static void OpenDoors(const char* id)
{
    if (!StatePlayer(std::string("DOORS_") + id)) return;      // conduits/shards have no zone doors
    if (GetFileAttributesW(L"C:\\Halcyon\\erebus_doormove.txt") != INVALID_FILE_ATTRIBUTES)      // test switch: replicated MOVEMENT
    {
        for (int n = 0; n < 4; ++n)
            if (SDK::AActor* d = A(Tg((std::string("EREBUS_DOOR_") + id + "_%d").c_str(), n)))
            { SDK::FVector p = d->K2_GetActorLocation(); p.Z += 1050.0; d->K2_SetActorLocation(p, false, nullptr, true); d->ForceNetUpdate(); }
        HxLog("[HalcyonA2][EREBUSDIR][STATE] doors %s moved server-side (replicated-movement test)\n", id);
        Sfx("door"); return;
    }
    if (PlayState(std::string("DOORS_") + id)) Sfx("door");
}

struct PlayerPts { SDK::AActor* pawn; SDK::FVector p[4]; };
static void Players(std::vector<PlayerPts>& out)
{
    static SDK::UClass* pawnCls = nullptr;
    if (!pawnCls) pawnCls = SDK::UObject::FindClassFast("VRPawn");
    if (!pawnCls) return;
    for (SDK::UObject* o : ClassObjects(pawnCls))
    {
        if (!o || o->IsDefaultObject()) continue;
        auto* pw = static_cast<SDK::AVRPawn*>(o);
        // real players only: skip unpossessed pawns and the server's own local player (the DLL keeps it alive)
        if (!pw->Controller || pw->Controller->IsLocalController()) continue;
        PlayerPts pp; pp.pawn = pw;
        pp.p[0] = pw->K2_GetActorLocation();
        pp.p[1] = pw->headLocal ? pw->headLocal->K2_GetComponentLocation() : pp.p[0];
        pp.p[2] = pw->LeftHandLocal ? pw->LeftHandLocal->K2_GetComponentLocation() : pp.p[1];
        pp.p[3] = pw->RightHandLocal ? pw->RightHandLocal->K2_GetComponentLocation() : pp.p[1];
        // no headset (flatscreen client) -> its tracked components report exactly (0,0,0): use the body instead
        for (int k = 1; k < 4; ++k)
            if (pp.p[k].X == 0.0 && pp.p[k].Y == 0.0 && pp.p[k].Z == 0.0) { pp.p[k] = pp.p[0]; pp.p[k].Z += (k == 1 ? 60.0 : 0.0); }
        out.push_back(pp);
    }
}
static bool NearAny(const std::vector<PlayerPts>& pl, const SDK::FVector& c, double r, bool handsOnly = false)
{
    for (const auto& p : pl) for (int k = handsOnly ? 2 : 0; k < 4; ++k) if (Dist(p.p[k], c) < r) return true;
    return false;
}
static SDK::FVector Up(SDK::AActor* a, double z) { SDK::FVector v = a->K2_GetActorLocation(); v.Z += z; return v; }

// Move a player. A2 movement is client-authoritative (Server_SetFrequentData overwrites a server-side teleport within
// a few frames - verified 2026-11-03), so ALSO ask the owning client to move itself via the game's own
// Client_HandleRestrictedMovementOfPlayer RPC (UA2PlayerTeamComponent; the King's Court kick-out path).
// Player positions are CLIENT-authoritative ([FDSYNC] copies the client's root onto the server pawn), so a server-side
// move is overwritten within frames. AVRPawn::Client_TeleportPawnWithLoading is A2's own server->owning-client RPC:
// the client teleports itself (with a short loading fade), then reports the new spot back as usual.
static bool Relocate(SDK::AActor* pawn, const SDK::FVector& to, double yawDeg = 0.0, float fade = 0.35f)
{
    static SDK::UClass* vrCls = SDK::UObject::FindClassFast("VRPawn");
    if (!vrCls || !pawn->IsA(vrCls)) { HxLog("[HalcyonA2][EREBUSDIR] relocate %s: not a VRPawn\n", pawn->GetName().c_str()); return false; }
    pawn->K2_SetActorLocation(to, false, nullptr, true);                      // server copy agrees until the client reports
    const SDK::FQuat q = SDK::UKismetMathLibrary::Quat_MakeFromEuler(SDK::FVector{ 0.0, 0.0, yawDeg });
    static_cast<SDK::AVRPawn*>(pawn)->Client_TeleportPawnWithLoading(to, q, SDK::FVector{ 0.0, 0.0, 0.0 }, fade, false);
    HxLog("[HalcyonA2][EREBUSDIR] relocate %s -> (%.0f,%.0f,%.0f) via Client_TeleportPawnWithLoading\n", pawn->GetName().c_str(), to.X, to.Y, to.Z);
    return true;
}

// Server-authoritative relocation: destroy the pawn and have the game mode restart the player at a transform, so the
// client possesses a fresh pawn there (candidate mechanism for beam zaps and fall recovery; under test 2026-11-03).
static bool Respawn(SDK::AActor* pawn, const SDK::FVector& to, double yawDeg)
{
    auto* p = static_cast<SDK::APawn*>(pawn);
    SDK::AController* ctrl = p->Controller;
    SDK::AGameModeBase* gm = SDK::UGameplayStatics::GetGameMode(pawn);
    if (!ctrl || !gm) { HxLog("[HalcyonA2][EREBUSDIR] respawn %s: no controller/gamemode\n", pawn->GetName().c_str()); return false; }
    SDK::FTransform tf{};
    tf.Rotation = SDK::UKismetMathLibrary::Quat_MakeFromEuler(SDK::FVector{ 0.0, 0.0, yawDeg });
    tf.Translation = to; tf.Scale3D = SDK::FVector{ 1.0, 1.0, 1.0 };
    const std::string old = pawn->GetName();
    pawn->K2_DestroyActor();
    gm->RestartPlayerAtTransform(ctrl, tf);
    SDK::APawn* np = SDK::UGameplayStatics::GetPlayerPawn(pawn, 0);
    HxLog("[HalcyonA2][EREBUSDIR] respawn %s -> (%.0f,%.0f,%.0f) via %s: controller now has %s\n", old.c_str(), to.X, to.Y, to.Z,
          gm->GetName().c_str(), static_cast<SDK::AController*>(ctrl)->K2_GetPawn() ? static_cast<SDK::AController*>(ctrl)->K2_GetPawn()->GetName().c_str() : "(no pawn)");
    (void)np;
    return true;
}

// Fall recovery ("never stuck"): world coordinates in cm, from zones_1_5.py (FZ=-278 m, garden GZ=FZ+12, chasm x 72..92 m,
// y 30..90 m, bottom FZ-20; reactor shaft bottom RB=FZ-24 is PLAYABLE - valve 0 sits there).
static SDK::FVector Checkpoint()
{
    int cur = 0; while (cur < kN && s_done[cur]) ++cur;
    if (cur <= 0 || cur >= kN) return SDK::FVector{ -17300.0, 900.0, -27650.0 };      // docking bay spawn
    // the doorway the players came through into the current zone (doors of the previous objective are open by now)
    for (int k = cur - 1; k >= 0; --k)
        if (SDK::AActor* d = A(std::string("EREBUS_DOOR_") + kChain[k] + "_0"))
        {
            SDK::FVector p = d->K2_GetActorLocation();
            if (SDK::ULevelSequencePlayer* sp = StatePlayer(std::string("DOORS_") + kChain[k])) (void)sp, p.Z -= 1050.0;  // door raised 10.5 m
            p.Z += 60.0;
            return p;
        }
    return SDK::FVector{ -17300.0, 900.0, -27650.0 };
}
static void FallTick(const std::vector<PlayerPts>& P)
{
    static std::unordered_map<void*, ULONGLONG> s_lastSave;
    const ULONGLONG now = GetTickCount64();
    for (const auto& pp : P)
    {
        const SDK::FVector& b = pp.p[0];
        const bool inChasm = b.X > 7200 && b.X < 9200 && b.Y > 3000 && b.Y < 9000 && b.Z < -26600.0 - 300.0;
        const bool outOfWorld = b.Z < -31000.0;
        if (!inChasm && !outOfWorld) continue;
        ULONGLONG& last = s_lastSave[pp.pawn];
        if (now - last < 3000) continue;                       // give the client time to arrive before re-checking
        last = now;
        const SDK::FVector to = inChasm ? SDK::FVector{ 6000.0, 6000.0, -26550.0 } : Checkpoint();
        HxLog("[HalcyonA2][EREBUSDIR][FALL] %s at (%.0f,%.0f,%.0f) %s -> recover\n", pp.pawn->GetName().c_str(), b.X, b.Y, b.Z,
              inChasm ? "in the garden chasm" : "out of the world");
        Relocate(pp.pawn, to, 0.0, 0.6f);
        Sfx("zap");
    }
}

static void Activate(int i)                                   // objective i becomes the current one
{
    if (i < 0 || i >= kN || s_started[i]) return;
    s_started[i] = true;
    const std::string id = kChain[i];
    if (id == "grid") Counter("EREBUS_TXT_GRID", -1, 0);
    else if (id == "valves") Counter("EREBUS_TXT_VALVES", -1, 0);
    else if (id == "cores") Counter("EREBUS_TXT_CORES", -1, 0);
    else if (id == "resonators") { Counter("EREBUS_TXT_RESO", -1, 0); Hide("EREBUS_TXT_RESO_ORDER", false); }
    HxLog("[HalcyonA2][EREBUSDIR] objective %s active\n", id.c_str());
}
static void Complete(int i)
{
    if (i < 0 || i >= kN || s_done[i]) return;
    s_done[i] = true;
    const std::string id = kChain[i];
    if (id == "grid") { for (SDK::AActor* b : s_beams) b->SetActorHiddenInGame(true); Counter("EREBUS_TXT_GRID", 0, 1); }
    if (id == "resonators") { for (int k = 0; k < 8; ++k) Hide(Tg("EREBUS_BRIDGE_%d", k), false); PlayState("BRIDGE"); }
    if (id == "shards") { Sfx("shardsdone"); Sfx("reward"); }
    OpenDoors(id.c_str());
    Sfx("done");
    HxLog("[HalcyonA2][EREBUSDIR] objective %s DONE\n", id.c_str());
    if (i + 1 < kN && kChain[i + 1] != std::string("conduits") && kChain[i + 1] != std::string("shards")) Activate(i + 1);
}

static void ResetState()
{
    for (auto& g : s_gates) { g.done = false; g.paused = false; }
    for (auto& c : s_cues) c.fired = false;
    for (int i = 0; i < kN; ++i) { s_done[i] = false; s_started[i] = false; }
    for (int i = 0; i < 4; ++i) { s_relayOn[i] = false; Hide(Tg("EREBUS_RELAY_%d_ON", i), true); Hide(Tg("EREBUS_RELAY_%d_OFF", i), false); }
    for (int i = 0; i < 3; ++i) { s_valveOn[i] = false; s_valveHold[i] = 0; Hide(Tg("EREBUS_VALVE_%d_ON", i), true); Hide(Tg("EREBUS_VALVE_%d_OFF", i), false); }
    for (int i = 0; i < 5; ++i) { s_core[i] = false; Hide(Tg("EREBUS_CORE_%d", i), false); }
    for (int i = 0; i < 4; ++i) { s_resoLit[i] = false; s_resoTouch[i] = false; Hide(Tg("EREBUS_RESO_%d_ON", i), true); Hide(Tg("EREBUS_RESO_%d_OFF", i), false); }
    for (int k = 0; k < 8; ++k) Hide(Tg("EREBUS_BRIDGE_%d", k), true);
    for (int i = 0; i < 6; ++i) { Hide(Tg("EREBUS_CONDUIT_%d_ON", i), true); Hide(Tg("EREBUS_CONDUIT_%d_OFF", i), false); }
    for (int i = 0; i < 30; ++i) { s_shard[i] = false; Hide(Tg("EREBUS_SHARD_%d", i), true); }
    for (SDK::AActor* b : s_beams) if (b->bHidden) b->SetActorHiddenInGame(false);
    CounterClear("EREBUS_TXT_RELAYS", 4); Counter("EREBUS_TXT_RELAYS", -1, 0);
    CounterClear("EREBUS_TXT_GRID", 1); CounterClear("EREBUS_TXT_VALVES", 3); CounterClear("EREBUS_TXT_CORES", 5);
    CounterClear("EREBUS_TXT_RESO", 4); Hide("EREBUS_TXT_RESO_ORDER", true);
    CounterClear("EREBUS_TXT_CONDUITS", 10); CounterClear("EREBUS_TXT_SHARDS", 30);
    s_resoStep = 0; s_charge = 0; s_chargeShown = -1; s_shardCount = 0; s_coreCount = 0; s_zapAt.clear();
    for (const char* id : kChain) RewindState(std::string("DOORS_") + id);
    RewindState("BRIDGE");
    s_started[0] = true;
    HxLog("[HalcyonA2][EREBUSDIR] objectives reset (door + bridge sequences rewound)\n");
}

static void Scan(SDK::ALevelSequenceActor* show)
{
    s_act.clear(); s_beams.clear(); s_gates.clear(); s_cues.clear(); s_sfx.clear();
    s_shardsAt = s_conduitsAt = 1e9;
    for (int i = 0; i < show->Tags.Num(); ++i)
    {
        const auto f = Split(show->Tags[i].GetRawString(), '|');
        if (f[0] == "EREBUS_GATE" && f.size() >= 4) { Gate g; g.id = f[1]; g.t = atof(f[2].c_str()); g.timeout = atof(f[3].c_str()); s_gates.push_back(g); }
        else if (f[0] == "EREBUS_CUE" && f.size() >= 3) { Cue c; c.t = atof(f[1].c_str()); c.path = f[2]; s_cues.push_back(c); }
        else if (f[0] == "EREBUS_SFX" && f.size() >= 3) s_sfx[f[1]] = f[2];
        else if (f[0] == "EREBUS_SHARDS_AT" && f.size() >= 2) s_shardsAt = atof(f[1].c_str());
        else if (f[0] == "EREBUS_CONDUITS_AT" && f.size() >= 2) s_conduitsAt = atof(f[1].c_str());
        else if (f[0] == "EREBUS_RESO_ORDER" && f.size() >= 5) for (int k = 0; k < 4; ++k) s_resoOrder[k] = atoi(f[1 + k].c_str());
    }
    static SDK::UClass* actCls = nullptr;
    if (!actCls) actCls = SDK::UObject::FindClassFast("Actor");
    if (actCls)
        for (SDK::UObject* o : ClassObjects(actCls))
        {
            if (!o || o->IsDefaultObject()) continue;
            auto* a = static_cast<SDK::AActor*>(o);
            if (a->Tags.Num() <= 0) continue;
            const std::string t = a->Tags[0].GetRawString();
            if (t.rfind("EREBUS_", 0) != 0) continue;
            if (t == "EREBUS_BEAM") s_beams.push_back(a); else s_act[t] = a;
            // The editor saved bReplicates but NOT the remote role, so every objective actor loaded with
            // RemoteRole=ROLE_None and the net driver never replicated its hidden/moved state (verified in game
            // 2026-11-03: doors stayed shut, crystals never lit on the client). Give them a proper remote role.
            // Setting the role alone was not enough (verified: server moved the door, client never saw it) - the actor
            // was never registered with the net driver because its role was None at load. SetReplicates(false->true)
            // is the engine's own path: it sets RemoteRole=SimulatedProxy and calls World->AddNetworkActor.
            if (a->bReplicates && a->RemoteRole == SDK::ENetRole::ROLE_None)
            {
                a->SetReplicates(false);
                a->SetReplicates(true);
                a->ForceNetUpdate();
            }
        }
    HxLog("[HalcyonA2][EREBUSDIR] show %s: %zu gates, %zu cues, %zu sfx, %zu objective actors, %zu beams\n",
          show->GetName().c_str(), s_gates.size(), s_cues.size(), s_sfx.size(), s_act.size(), s_beams.size());
    {   // A2's replication graph routes actors BY CLASS - log which classes it routes (one-shot)
        static bool s_rgLogged = false;
        static SDK::UClass* rgCls = SDK::UObject::FindClassFast("A2ReplicationGraph");
        if (!s_rgLogged && rgCls)
            for (SDK::UObject* o : ClassObjects(rgCls))
            {
                if (!o || o->IsDefaultObject()) continue;
                s_rgLogged = true;
                auto* rg = static_cast<SDK::UA2ReplicationGraph*>(o);
                auto dump = [](const char* n, SDK::TArray<SDK::UClass*>& arr) {
                    std::string s; for (int i = 0; i < arr.Num(); ++i) if (arr[i]) { s += arr[i]->GetName(); s += ' '; }
                    HxLog("[HalcyonA2][EREBUSDIR][REPGRAPH] %s (%d): %s\n", n, arr.Num(), s.c_str());
                };
                dump("Spatialized", rg->SpatializedClasses); dump("NonSpatialized", rg->NonSpatializedClasses);
                dump("AlwaysRelevant", rg->AlwaysRelevantClasses);
            }
    }
    for (const char* t : { "EREBUS_DOOR_relays_0", "EREBUS_RELAY_0_ON", "EREBUS_TXT_RELAYS_0" })   // replication diagnostics
        if (SDK::AActor* a = A(t))
            HxLog("[HalcyonA2][EREBUSDIR][REP] %s (%s): bReplicates=%d RemoteRole=%d Role=%d bRepMove=%d bAlwaysRelevant=%d "
                  "bNetLoadOnClient=%d dormancy=%d cullSq=%.0f hidden=%d\n", t, a->GetName().c_str(), a->bReplicates, (int)a->RemoteRole,
                  (int)a->Role, a->bReplicateMovement, a->bAlwaysRelevant, a->bNetLoadOnClient, (int)a->NetDormancy,
                  a->NetCullDistanceSquared, a->bHidden);
}

static void SkipCurrent()
{
    for (int i = 0; i < kN; ++i)
    {
        if (s_done[i]) continue;
        const std::string id = kChain[i];
        if (id == "relays") for (int k = 0; k < 4; ++k) { Hide(Tg("EREBUS_RELAY_%d_OFF", k), true); Hide(Tg("EREBUS_RELAY_%d_ON", k), false); }
        if (id == "shards") for (int k = 0; k < 30; ++k) Hide(Tg("EREBUS_SHARD_%d", k), true);
        HxLog("[HalcyonA2][EREBUSDIR] SKIP %s\n", id.c_str());
        Complete(i);
        return;
    }
}

static void Tick()
{
    static ULONGLONG s_last = 0; const ULONGLONG now = GetTickCount64();
    if (now - s_last < 100) return;
    const double dt = s_last ? std::min(0.5, (now - s_last) / 1000.0) : 0.1;
    s_last = now;
    static ULONGLONG s_fileChk = 0;
    if (now - s_fileChk > 1000)
    {
        s_fileChk = now;
        if (GetFileAttributesW(L"C:\\Halcyon\\erebus_skip.txt") != INVALID_FILE_ATTRIBUTES) { DeleteFileW(L"C:\\Halcyon\\erebus_skip.txt"); s_skipReq = true; }
    }
    SDK::ALevelSequenceActor* show = ShowActor();
    if (!show) { s_show = nullptr; return; }
    if (show != s_show) { s_show = show; Scan(show); ResetState(); }
    if (s_gates.empty() && s_cues.empty()) return;
    if (s_skipReq) { s_skipReq = false; SkipCurrent(); }
    SDK::ULevelSequencePlayer* pl = show->SequencePlayer;
    if (!pl) return;
    static SDK::UFunction* fnTime = nullptr;      // windows.h #defines GetCurrentTime -> call the UFunction directly
    if (!fnTime && pl->Class) fnTime = pl->Class->GetFunction("MovieSceneSequencePlayer", "GetCurrentTime");
    if (!fnTime) return;
    struct { SDK::FQualifiedFrameTime ReturnValue; } tp{};
    if (!SafeProcessEvent(pl, fnTime, &tp)) return;
    const SDK::FQualifiedFrameTime qt = tp.ReturnValue;
    const double t = qt.Rate.Numerator ? (qt.Time.FrameNumber.Value + qt.Time.SubFrame) * qt.Rate.Denominator / double(qt.Rate.Numerator) : 0.0;
    const bool playing = pl->IsPlaying();

    for (auto& c : s_cues)
    {
        if (c.fired && t + 1.0 < c.t) c.fired = false;
        if (!c.fired && playing && t >= c.t && t < c.t + 3.0) { c.fired = true; Sound(c.path); }
        else if (!c.fired && t >= c.t + 3.0) c.fired = true;
    }

    // [TEST ONLY, -ErebusTest] C:\Halcyon\erebus_tp.txt = "x y z" (cm) or "tag <EREBUS_TAG> [zOffset]": teleport the
    // first real player there, so a flatscreen tester can walk the objective chain. Off on normal servers.
    // Runs BEFORE positions are sampled so this tick's checks see the new spot.
    static const bool s_testMode = wcsstr(GetCommandLineW(), L"-ErebusTest") != nullptr;
    std::vector<PlayerPts> P; Players(P);
    FallTick(P);
    {   // late join -> 4 s later re-broadcast every paused sequence (show at a gate, finished door/bridge states)
        static std::unordered_map<void*, ULONGLONG> s_seen; static ULONGLONG s_resyncAt = 0;
        for (const auto& pp : P) if (s_seen.emplace(pp.pawn, now).second) { s_resyncAt = now + 4000; HxLog("[HalcyonA2][EREBUSDIR] player joined: %s\n", pp.pawn->GetName().c_str()); }
        if (s_resyncAt && now >= s_resyncAt)
        {
            s_resyncAt = 0;
            {   // [REPGRAPH] which replication driver is live once a client is connected (one line per instance + CDO lists)
                static SDK::UClass* rdCls = SDK::UObject::FindClassFast("ReplicationDriver");
                static SDK::UClass* rgCls = SDK::UObject::FindClassFast("A2ReplicationGraph");
                int nInst = 0;
                if (rdCls)
                    for (SDK::UObject* o : ClassObjects(rdCls))
                    {
                        if (!o) continue;
                        HxLog("[HalcyonA2][EREBUSDIR][REPGRAPH] driver %s class=%s cdo=%d\n", o->GetName().c_str(), o->Class ? o->Class->GetName().c_str() : "?", o->IsDefaultObject() ? 1 : 0);
                        if (!o->IsDefaultObject()) ++nInst;
                        if (rgCls && o->IsA(rgCls))
                        {
                            auto* rg = static_cast<SDK::UA2ReplicationGraph*>(o);
                            auto dump = [](const char* n, SDK::TArray<SDK::UClass*>& arr) {
                                std::string s; for (int i = 0; i < arr.Num(); ++i) if (arr[i]) { s += arr[i]->GetName(); s += ' '; }
                                HxLog("[HalcyonA2][EREBUSDIR][REPGRAPH]   %s (%d): %s\n", n, arr.Num(), s.c_str());
                            };
                            dump("Spatialized", rg->SpatializedClasses); dump("NonSpatialized", rg->NonSpatializedClasses);
                            dump("AlwaysRelevant", rg->AlwaysRelevantClasses);
                        }
                    }
                HxLog("[HalcyonA2][EREBUSDIR][REPGRAPH] %d live replication driver instance(s)%s\n", nInst, nInst ? "" : " -> plain UNetDriver replication");
            }
            if (GetFileAttributesW(L"C:\\Halcyon\\erebus_noresync_show.txt") == INVALID_FILE_ATTRIBUTES) ResyncPaused(pl, "show");   // file = test bisect switch
            else HxLog("[HalcyonA2][EREBUSDIR][STATE] show resync skipped (test switch)\n");
            for (const char* id : kChain) ResyncPaused(StatePlayer(std::string("DOORS_") + id), id);
            ResyncPaused(StatePlayer("BRIDGE"), "bridge");
        }
    }
    static SDK::FVector s_pinTo{}; static ULONGLONG s_pinUntil = 0;           // "hold <secs> ..." keeps the tester there
    if (s_testMode && !P.empty() && now < s_pinUntil)
    {
        P[0].pawn->K2_SetActorLocation(s_pinTo, false, nullptr, true);
        P.clear(); Players(P);
    }
    if (s_testMode && !P.empty())
    {
        FILE* f = nullptr;
        if (_wfopen_s(&f, L"C:\\Halcyon\\erebus_tp.txt", L"r") == 0 && f)
        {
            char buf[256]{}; fgets(buf, sizeof buf, f); fclose(f); DeleteFileW(L"C:\\Halcyon\\erebus_tp.txt");
            double holdS = 0; char* req = buf; bool respawn = false;
            if (sscanf_s(buf, "hold %lf", &holdS) == 1) { req = strchr(buf + 5, ' '); req = req ? req + 1 : buf; }
            if (strncmp(req, "respawn ", 8) == 0) { respawn = true; req += 8; }
            SDK::FVector to{}; bool ok = false;
            char tagn[128]{}; double zo = 0;
            if (sscanf_s(req, "tag %127s %lf", tagn, (unsigned)sizeof tagn, &zo) >= 1)
            { if (SDK::AActor* a = A(tagn)) { to = a->K2_GetActorLocation(); to.Z += zo; ok = true; } }
            else ok = sscanf_s(req, "%lf %lf %lf", &to.X, &to.Y, &to.Z) == 3;
            if (ok)
            {
                // Respawn is DISABLED: destroying the pawn leaves the client body-less - A2's game mode never spawns a
                // new one (verified 2026-11-03). Player relocation must come from a client-side mechanism.
                if (respawn) HxLog("[HalcyonA2][EREBUSDIR][TEST] respawn disabled (destroys the player's pawn)\n");
                else Relocate(P[0].pawn, to);
                s_pinTo = to; s_pinUntil = holdS > 0 ? now + static_cast<ULONGLONG>(holdS * 1000.0) : 0;
                P.clear(); Players(P);                                       // re-sample after the move
                HxLog("[HalcyonA2][EREBUSDIR][TEST] teleport served; server now sees (%.0f,%.0f,%.0f)\n",
                      P.empty() ? 0.0 : P[0].p[0].X, P.empty() ? 0.0 : P[0].p[0].Y, P.empty() ? 0.0 : P[0].p[0].Z);
            }
            else HxLog("[HalcyonA2][EREBUSDIR][TEST] bad teleport request: %s\n", buf);
        }
    }
    const int need = std::max(1, int((P.size() + 1) / 2));
    static ULONGLONG s_diagAt = 0;                                           // what the SERVER sees of each player
    if (now - s_diagAt > 5000)
    {
        s_diagAt = now;
        HxLog("[HalcyonA2][EREBUSDIR] t=%.1fs playing=%d players=%zu\n", t, playing ? 1 : 0, P.size());
        for (const auto& p : P)
        {
            auto* pw = static_cast<SDK::AVRPawn*>(p.pawn);
            HxLog("[HalcyonA2][EREBUSDIR]   %s actor(%.0f,%.0f,%.0f) head(%.0f,%.0f,%.0f)%s L(%.0f,%.0f,%.0f)%s R(%.0f,%.0f,%.0f)%s\n",
                  p.pawn->GetName().c_str(), p.p[0].X, p.p[0].Y, p.p[0].Z, p.p[1].X, p.p[1].Y, p.p[1].Z, pw->headLocal ? "" : "[no comp]",
                  p.p[2].X, p.p[2].Y, p.p[2].Z, pw->LeftHandLocal ? "" : "[no comp]", p.p[3].X, p.p[3].Y, p.p[3].Z, pw->RightHandLocal ? "" : "[no comp]");
        }
    }

    if (s_started[0] && !s_done[0])                                         // relays
    {
        int on = 0;
        for (int i = 0; i < 4; ++i)
        {
            SDK::AActor* r = A(Tg("EREBUS_RELAY_%d", i));
            if (!s_relayOn[i] && r && NearAny(P, Up(r, 100.0), 220.0))
            {
                s_relayOn[i] = true; Hide(Tg("EREBUS_RELAY_%d_OFF", i), true); Hide(Tg("EREBUS_RELAY_%d_ON", i), false);
                int n = 0; for (bool b : s_relayOn) n += b;
                Counter("EREBUS_TXT_RELAYS", n - 1, n); Sfx("relay");
            }
            on += s_relayOn[i];
        }
        if (on >= 4) Complete(0);
    }
    if (s_started[1] && !s_done[1])                                         // grid
    {
        SDK::AActor* end = A("EREBUS_GRID_END");
        const double endX = end ? end->K2_GetActorLocation().X - 600.0 : -1000.0;
        int past = 0;
        for (const auto& p : P)
        {
            if (p.p[1].X >= endX) ++past;
            for (SDK::AActor* b : s_beams)
            {
                const SDK::FVector bl = b->K2_GetActorLocation();
                bool hit = false;
                for (int k = 1; k < 4 && !hit; ++k)
                    hit = fabs(p.p[k].X - bl.X) < 35.0 && fabs(p.p[k].Z - bl.Z) < 35.0 && fabs(p.p[k].Y - bl.Y) < 1600.0;
                if (!hit) continue;
                ULONGLONG& last = s_zapAt[p.pawn];
                if (now - last < 2500) break;
                last = now;
                Relocate(p.pawn, SDK::FVector{ -11600.0, 0.0, -27700.0 });
                Sfx("zap");
                HxLog("[HalcyonA2][EREBUSDIR] %s touched a beam -> back to the concourse entrance\n", p.pawn->GetName().c_str());
                break;
            }
        }
        if (past >= need) Complete(1);
    }
    if (s_started[2] && !s_done[2])                                         // valves: hold 3 s each
    {
        int on = 0;
        for (int i = 0; i < 3; ++i)
        {
            SDK::AActor* v = A(Tg("EREBUS_VALVE_%d", i));
            if (!s_valveOn[i] && v)
            {
                if (NearAny(P, Up(v, 100.0), 200.0, true)) s_valveHold[i] += dt; else s_valveHold[i] = std::max(0.0, s_valveHold[i] - dt);
                if (s_valveHold[i] >= 3.0)
                {
                    s_valveOn[i] = true; Hide(Tg("EREBUS_VALVE_%d_OFF", i), true); Hide(Tg("EREBUS_VALVE_%d_ON", i), false);
                    int n = 0; for (bool b : s_valveOn) n += b;
                    Counter("EREBUS_TXT_VALVES", n - 1, n); Sfx("relay");
                }
            }
            on += s_valveOn[i];
        }
        if (on >= 3) Complete(2);
    }
    if (s_started[3] && !s_done[3])                                         // data cores
    {
        for (int i = 0; i < 5; ++i)
        {
            SDK::AActor* c = A(Tg("EREBUS_CORE_%d", i));
            if (s_core[i] || !c || !NearAny(P, c->K2_GetActorLocation(), 140.0, true)) continue;
            s_core[i] = true; c->SetActorHiddenInGame(true);
            Counter("EREBUS_TXT_CORES", s_coreCount, s_coreCount + 1); ++s_coreCount; Sfx("shard");
        }
        if (s_coreCount >= 5) Complete(3);
    }
    if (s_started[4] && !s_done[4])                                         // resonators: in order
    {
        for (int i = 0; i < 4; ++i)
        {
            SDK::AActor* r = A(Tg("EREBUS_RESO_%d", i));
            const bool touch = r && NearAny(P, Up(r, 150.0), 220.0, true);
            const bool edge = touch && !s_resoTouch[i];
            s_resoTouch[i] = touch;
            if (!edge || s_resoLit[i]) continue;
            if (i == s_resoOrder[s_resoStep])
            {
                s_resoLit[i] = true; Hide(Tg("EREBUS_RESO_%d_OFF", i), true); Hide(Tg("EREBUS_RESO_%d_ON", i), false);
                Counter("EREBUS_TXT_RESO", s_resoStep, s_resoStep + 1); ++s_resoStep; Sfx("conduit");
            }
            else
            {
                for (int k = 0; k < 4; ++k) { s_resoLit[k] = false; Hide(Tg("EREBUS_RESO_%d_ON", k), true); Hide(Tg("EREBUS_RESO_%d_OFF", k), false); }
                Counter("EREBUS_TXT_RESO", s_resoStep, 0); s_resoStep = 0; Sfx("zap");
                HxLog("[HalcyonA2][EREBUSDIR] resonators: wrong order -> reset\n");
            }
        }
        if (s_resoStep >= 4) Complete(4);
    }
    if (s_done[4] && !s_done[5] && t >= s_conduitsAt)                        // conduits
    {
        if (!s_started[5]) { Activate(5); s_chargeShown = 0; Counter("EREBUS_TXT_CONDUITS", -1, 0); }
        int held = 0;
        for (int i = 0; i < 6; ++i)
        {
            SDK::AActor* c = A(Tg("EREBUS_CONDUIT_%d", i));
            const bool h = c && NearAny(P, c->K2_GetActorLocation(), 380.0);
            held += h;
            Hide(Tg("EREBUS_CONDUIT_%d_ON", i), !h); Hide(Tg("EREBUS_CONDUIT_%d_OFF", i), h);
        }
        const int needC = std::max(1, std::min<int>(6, int(P.size())));
        s_charge += dt * (held >= needC ? 1.0 / 20.0 : 0.25 / 20.0 * held / needC);
        const int k = std::min(10, int(s_charge * 10.0));
        if (k != s_chargeShown) { Counter("EREBUS_TXT_CONDUITS", s_chargeShown, k); s_chargeShown = k; Sfx("conduit"); }
        if (s_charge >= 1.0) Complete(5);
    }
    if (!s_done[6] && t >= s_shardsAt)                                       // shards (deck + atrium)
    {
        if (!s_started[6])
        {
            Activate(6);
            CounterClear("EREBUS_TXT_CONDUITS", 10);
            Counter("EREBUS_TXT_SHARDS", -1, 0);
            for (int i = 0; i < 30; ++i) Hide(Tg("EREBUS_SHARD_%d", i), false);
        }
        for (int i = 0; i < 30; ++i)
        {
            if (s_shard[i]) continue;
            SDK::AActor* s = A(Tg("EREBUS_SHARD_%d", i));
            if (!s || !NearAny(P, s->K2_GetActorLocation(), 130.0, true)) continue;
            s_shard[i] = true; s->SetActorHiddenInGame(true);
            Counter("EREBUS_TXT_SHARDS", s_shardCount, s_shardCount + 1); ++s_shardCount; Sfx("shard");
        }
        if (s_shardCount >= 30) Complete(6);
    }

    for (auto& g : s_gates)                                                  // hold the show at each gate
    {
        if (g.done) continue;
        const int oi = Idx(g.id);
        if (oi < 0 || s_done[oi])
        {
            g.done = true;
            if (g.paused) { g.paused = false; pl->Play(); HxLog("[HalcyonA2][EREBUSDIR] gate %s cleared -> show resumes\n", g.id.c_str()); }
            continue;
        }
        if (!g.paused && playing && t >= g.t - 0.05)
        {
            pl->Pause(); g.paused = true; g.pausedAt = now;
            HxLog("[HalcyonA2][EREBUSDIR] gate %s: show paused at %.1fs (timeout %.0fs)\n", g.id.c_str(), t, g.timeout);
        }
        else if (g.paused && now - g.pausedAt > static_cast<ULONGLONG>(g.timeout * 1000.0))
        {
            HxLog("[HalcyonA2][EREBUSDIR] gate %s timed out -> objective forced done\n", g.id.c_str());
            SkipCurrent();
        }
        break;
    }
}
}   // namespace ebdir
