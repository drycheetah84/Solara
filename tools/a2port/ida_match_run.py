import json, traceback
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
try:
    src=open(A+r"\ida_match.py").read().replace(r'\new-map.json', r'\new-map-selftest.json')
    exec(src)
    open(A+r"\match_ok.txt","w").write("ok")
except Exception:
    open(A+r"\match_err.txt","w").write(traceback.format_exc())
