# cpp-escape-via-heredoc

> Patching C/C++ source through a bash heredoc + python turns \"\\\\n\" in string literals into real newlines (C2001)
>
> Type: feedback. Written by Claude during sessions with the project owner ("the user" = the owner). Paths under `C:\Users\drych\` are on the owner's PC. Verify status lines against the code before acting.

When I patch HalcyonA2 sources (dllmain_*.cpp / specedit.h) by piping a Python script through a Bash heredoc,
a `\\n` meant for a C string literal arrives as a real newline, splitting the literal ("newline in string
literal", C2001). It happened 5+ times on 2026-10-31.

**Why:** the escape is consumed somewhere between the tool, bash and Python, so the file ends up with a real line break.

**How to apply:** for C escapes, build them with `chr(92)+'n'` inside the Python code, or use the Edit tool for
the line. After any scripted patch that adds an HxLog/printf, check it with
`sed -n Np file | cat -A` before building. A join pass that merges `HxLog("...` + `", args` lines fixes it.
