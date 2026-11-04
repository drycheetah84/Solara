# a2-robot-animation

> How to animate the A2 player robot (Creator Kit rig) so it looks good: own two-bone IK on hand/head paths, not bone keyframes or Blender IK
>
> Type: project. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

The user cannot animate; Claude makes event animations. A2 has NO authored body animations (VR game: robots are
IK-driven from head + hands), so motion must be made. Rig = official Creator Kit
`C:\Users\drych\Downloads\CreatorKit\Models\RobotD\SK_A2RobotD_LOD0.blend` (38 bones, identical names/order/hierarchy to
the game's `/Game/A2/Characters/A2RobotD/SK_A2RobotD_Skeleton`, which also has 30 sockets the .blend lacks). Albedo
`T_A2RobotD_ALB.tga`. Blender 5.2 at `C:\Program Files\Blender Foundation\Blender 5.2\blender.exe` (no PIL in py; stitch
previews with PowerShell System.Drawing; video needs image_settings.media_type='VIDEO' then FFMPEG). UE 5.5.x needed
(game = Unreal Engine 5.5.3; user installed 5.5.4).

**What works (v2 wave, 2026-11-03: clean in preview renders; user verdict pending):** animate hand/head TARGET paths (Catmull-Rom splines, anticipation + overshoot),
solve arms with our OWN two-bone IK from joint positions: shoulder=ArmX head, elbow=ForearmUpperX head, wrist=HandX head,
pole out/back/down; set `pose_bone.matrix` (armature space, pivot rotations) and view_layer.update() between bones; Root
bone for float bob/lean. Script: scratchpad `anim/wave_ik2.py` (regenerate if scratchpad is gone).

**Hands:** orient explicitly (fingers = mean IndexA/MiddleA head - Hand head, thumb toward body), but along the FOREARM
direction (straight wrist, small flick), never world-up: user flagged "hand going backwards" (inherited fold) and then
"hand isn't moving with the arm" (world-locked). A wave = forearm arcing around a fixed elbow (`anim/wave_ik4.py`).

**What failed:** (1) per-bone world-axis keyframes = stiff straight arm ("god awful"); (2) Blender IK constraint =
twisted/snapped forearms, because this rig's bone tails do NOT point at their children.

Robot faces -Y, its right is -X, root at head top, body hangs to z≈-0.95 m. Related: [a2-50451-christmas-port](a2-50451-christmas-port.md).
