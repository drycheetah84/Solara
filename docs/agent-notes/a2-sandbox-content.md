# A2 sandbox content (Axiom S3 bucket) — what the game servers fetch

> Type: project. Written by Claude, 2026-11-07. Verify against the code before acting.

**Bucket:** `https://a2-sandboxprojects.s3.us-east-2.amazonaws.com/` (exe: `UAmazonAWSCloud`, host string
`a2-sandboxprojects` + `.amazonaws.com/`; it also signs via `sts.us-east-2.amazonaws.com`). Anonymous GET works for
every object below; anonymous LIST is 403, so this inventory comes from what servers requested plus references
inside those files.

**Why it matters:** the server's built-in libcurl CA store cannot verify Amazon's certificate (libcurl error 60,
"unable to get local issuer certificate"), so before 2026-11-07 EVERY fetch failed: ScrapRun's trap bundle
("Failed to fetch cloud config with GUID 065BB…", then "Skipping Fetch request … already being fetched" forever)
-> no traps spawned / traps gone after a round; plus every cloud sandbox gamemode (October districts, BlockBuster,
Smoosh_Gear, KazzPKR_Progression25 ...). Find it in `C:\Windows\System32\config\systemprofile\AppData\Local\A2\Saved\Logs\A2*.log`
on the game boxes (NOT HalcyonA2.log).

**Fix (c5df4bb, deployed 2026-11-07 01:35):** DLL `[CURLREDIRECT]` rewrites `a2-sandboxprojects` URLs to
`http://127.0.0.1:78/sandboxprojects/<path>` (box portproxy -> hub). Solara `internal/ares/sandbox_proxy.go` fetches
from S3 and caches under `/opt/solara/sandbox_cache/<path>` (200s only), so content survives the bucket going away.
Back up that folder.

**Path forms:** `configs/v1/<id>/config.json` (trap bundles), `projects/v1/<id>/info.json` (has `latestVersion`),
`releases/v1/<id>/<semver>.aarel` (cloud gamemode, ProjectSource `1;c;g;<id>;;^<ver>`),
`gamemodes/v2/<id>/<latestVersion>.aaprj` (ProjectSource `1;f;g;<id>;<timestamp>`), `quests/v2/<id>/<n>.aaqst`.
`directories/v1/<id>/info.json` returns 404 for these ids.

**Inventory (62 objects, 2.7 MB, all cached):**

| Kind | Id | Name | Type | Bytes |
|---|---|---|---|---|
| quests/v2 | `F52654EA4CA7A04E166C53AF490420EA` | ? | quests | 293424 |
| projects/v1 | `2CA6D3E548B005E88D76749F2CA6DF20` | BlockBuster | Gamemode | 7861 |
| releases/v1 | `2CA6D3E548B005E88D76749F2CA6DF20` | BlockBuster | Gamemode | 95144 |
| projects/v1 | `C0C24E934817657006DD81967D2B89A4` | BlockBusterShell | Gamemode | 1251 |
| releases/v1 | `C0C24E934817657006DD81967D2B89A4` | BlockBusterShell | Gamemode | 2244 |
| projects/v1 | `9FB29BDE49D124631A1DEA8D3495BD58` | Bunny_Hop_Corner | Gamemode | 2391 |
| releases/v1 | `9FB29BDE49D124631A1DEA8D3495BD58` | Bunny_Hop_Corner | Gamemode | 8435 |
| projects/v1 | `9BD73DC945DE31125CBE6ABCA711F074` | Carve_Corner | Gamemode | 1247 |
| releases/v1 | `9BD73DC945DE31125CBE6ABCA711F074` | Carve_Corner | Gamemode | 8739 |
| projects/v1 | `9C40B9B74E78653F3553BAB3832DE3BD` | Ceiling_Pillars | Gamemode | 4109 |
| releases/v1 | `9C40B9B74E78653F3553BAB3832DE3BD` | Ceiling_Pillars | Gamemode | 10622 |
| projects/v1 | `C0462494409A741239FED1B2114FE435` | Crank_Side_Balance_Beam | Gamemode | 4061 |
| releases/v1 | `C0462494409A741239FED1B2114FE435` | Crank_Side_Balance_Beam | Gamemode | 5615 |
| projects/v1 | `CB9F3DB644466DA42E9FB98785E3E491` | Double_Walls_Corner | Gamemode | 969 |
| releases/v1 | `CB9F3DB644466DA42E9FB98785E3E491` | Double_Walls_Corner | Gamemode | 5795 |
| projects/v1 | `4080239C471933822CFA42B1EDB696FD` | Flipping_Plats | Gamemode | 3820 |
| releases/v1 | `4080239C471933822CFA42B1EDB696FD` | Flipping_Plats | Gamemode | 11174 |
| configs/v1 | `065BB1094A429390370B6B9AB1B4CB4D` | Frenzy Fest | trap bundle | 2806 |
| projects/v1 | `78CDA0DF4C08167E968ADBB4BEEA87EB` | Grab_Corner_Checkpoint | Gamemode | 1257 |
| releases/v1 | `78CDA0DF4C08167E968ADBB4BEEA87EB` | Grab_Corner_Checkpoint | Gamemode | 8783 |
| projects/v1 | `03E24A154AC37E0B4D5047BDD975E375` | Gravity_Controller | Gamemode | 2108 |
| releases/v1 | `03E24A154AC37E0B4D5047BDD975E375` | Gravity_Controller | Gamemode | 80720 |
| projects/v1 | `B6B26C11404FB4F67920BCABF2883563` | HB Club October | Gamemode | 12369 |
| releases/v1 | `B6B26C11404FB4F67920BCABF2883563` | HB Club October | Gamemode | 93055 |
| projects/v1 | `02B256E940C2E136A57E26A4F1DBC80A` | HB Driftball October | Gamemode | 12751 |
| releases/v1 | `02B256E940C2E136A57E26A4F1DBC80A` | HB Driftball October | Gamemode | 296444 |
| projects/v1 | `7BB7D8CD40119139511B0BBFF1BFF123` | HB Parkour October | Gamemode | 37641 |
| releases/v1 | `7BB7D8CD40119139511B0BBFF1BFF123` | HB Parkour October | Gamemode | 618500 |
| gamemodes/v2 | `E0DA56394D856A22821DF6856232877D` | KazzPKR_Progression25 | Gamemode | 748333 |
| projects/v1 | `E0DA56394D856A22821DF6856232877D` | KazzPKR_Progression25 | Gamemode | 39419 |
| projects/v1 | `13D585C045691104F6DD85A4D96491DC` | Moving_Monkey_Bars | Gamemode | 2963 |
| releases/v1 | `13D585C045691104F6DD85A4D96491DC` | Moving_Monkey_Bars | Gamemode | 13563 |
| projects/v1 | `B49D56E6447CA8A8EBE5DBA307EFE70F` | Playground_October | Gamemode | 13235 |
| releases/v1 | `B49D56E6447CA8A8EBE5DBA307EFE70F` | Playground_October | Gamemode | 10831 |
| projects/v1 | `841AC3F3456BC5676DD81A9B76E7B609` | Playground_October_2_Bouncewalls | Gamemode | 2481 |
| releases/v1 | `841AC3F3456BC5676DD81A9B76E7B609` | Playground_October_2_Bouncewalls | Gamemode | 6337 |
| projects/v1 | `59ED98E84B68CEC33A09E38C53674B63` | RC_Funnel | Gamemode | 4103 |
| releases/v1 | `59ED98E84B68CEC33A09E38C53674B63` | RC_Funnel | Gamemode | 18824 |
| projects/v1 | `9AF887E740520E5FEED6E4B96F11B9FB` | Revolving_Doors | Gamemode | 6357 |
| releases/v1 | `9AF887E740520E5FEED6E4B96F11B9FB` | Revolving_Doors | Gamemode | 14842 |
| projects/v1 | `65C86D3046E3CB621F94AAA067836645` | Row_Ball_Cannons | Gamemode | 5782 |
| releases/v1 | `65C86D3046E3CB621F94AAA067836645` | Row_Ball_Cannons | Gamemode | 10102 |
| gamemodes/v2 | `706B904B43C4B1CA180310A1B8DA65C5` | ScraprunEndTrap | Gamemode | 12641 |
| projects/v1 | `706B904B43C4B1CA180310A1B8DA65C5` | ScraprunEndTrap | Gamemode | 4446 |
| gamemodes/v2 | `AC79E27945E5CDE47687049BB6093367` | ScraprunStartArea | Gamemode | 3607 |
| projects/v1 | `AC79E27945E5CDE47687049BB6093367` | ScraprunStartArea | Gamemode | 1518 |
| projects/v1 | `3C258B0A4B3DCC0F05F97194D3CA5305` | slip_Floor_Corner | Gamemode | 2962 |
| releases/v1 | `3C258B0A4B3DCC0F05F97194D3CA5305` | slip_Floor_Corner | Gamemode | 6453 |
| projects/v1 | `01C4FB2048D7DDF90A6A51B23FFD8589` | Smoosh_Gear | Gamemode | 7505 |
| releases/v1 | `01C4FB2048D7DDF90A6A51B23FFD8589` | Smoosh_Gear | Gamemode | 13200 |
| projects/v1 | `C5A32C5E452A447495AF71A6EB52D094` | Spinning_Circles | Gamemode | 2961 |
| releases/v1 | `C5A32C5E452A447495AF71A6EB52D094` | Spinning_Circles | Gamemode | 6849 |
| projects/v1 | `DC4523564829BD9F84AE628492844383` | Spinnng_Plats_With_Bombs | Gamemode | 2969 |
| releases/v1 | `DC4523564829BD9F84AE628492844383` | Spinnng_Plats_With_Bombs | Gamemode | 13337 |
| projects/v1 | `AC32F255402E4E9FA8E9499B4D1AFA31` | Start_Beam_Cylinders | Gamemode | 1858 |
| releases/v1 | `AC32F255402E4E9FA8E9499B4D1AFA31` | Start_Beam_Cylinders | Gamemode | 15969 |
| projects/v1 | `899A5E1842C2FE6E3A07C8A9429DAC17` | Step_Stones_Checkpoint | Gamemode | 2682 |
| releases/v1 | `899A5E1842C2FE6E3A07C8A9429DAC17` | Step_Stones_Checkpoint | Gamemode | 11498 |
| projects/v1 | `DE98A51A4DB93C687546F791D6469D5F` | Swinging_Bars | Gamemode | 1248 |
| releases/v1 | `DE98A51A4DB93C687546F791D6469D5F` | Swinging_Bars | Gamemode | 9695 |
| projects/v1 | `BFDDF62A4BA89B0D062F5AABC0F6B2C8` | Triangle_Platforms | Gamemode | 4400 |
| releases/v1 | `BFDDF62A4BA89B0D062F5AABC0F6B2C8` | Triangle_Platforms | Gamemode | 6446 |
