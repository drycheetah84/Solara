import json, traceback, idaapi, idautils, idc, ida_funcs
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
log=open(A+r"\sent.txt","w")
def p(*a): log.write(" ".join(str(x) for x in a)+"\n"); log.flush()
try:
    p("start ib", hex(idaapi.get_imagebase()))
    n=0
    for ea in idautils.Functions():
        n+=1
    p("functions", n)
    cnt=0
    for s in idautils.Strings():
        cnt+=1
        if cnt>5: break
    p("strings iter ok sample", cnt)
    p("old-ida.json exists", __import__('os').path.exists(A+r"\old-ida.json"))
    p("DONE")
except Exception:
    p("ERR", traceback.format_exc())
log.close()
