import traceback
try:
    exec(open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\ida_match_selftest.py").read())
except Exception:
    open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\match_err.txt","w").write(traceback.format_exc())
