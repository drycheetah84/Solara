import idaapi, idc, ida_funcs, ida_hexrays, ida_pro, json, re, bisect, difflib
idaapi.auto_wait()
ib=idaapi.get_imagebase()
W=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
A=json.load(open(W+r"\anchors_47031.json"))
B=json.load(open(W+r"\bodies47.json"))
C=json.load(open(W+r"\confirm2.json"))
conf=C["confirmed"]
# GetNetMode manual anchor (confirmed out of band)
conf.setdefault("41F5A60",{"rva50":"0x41f5a20"})

def norm(s):
    if not s: return ""
    s=re.sub(r'sub_[0-9A-Fa-f]+','F',s)
    s=re.sub(r'(byte|word|dword|qword|unk|off|loc|stru|xmmword|flt|dbl)_[0-9A-Fa-f]+','G',s)
    s=re.sub(r'0x[0-9A-Fa-f]+','N',s)
    s=re.sub(r'\b[0-9]+\b','N',s)
    s=re.sub(r'\bv[0-9]+\b','v',s); s=re.sub(r'\ba[0-9]+\b','a',s)
    s=re.sub(r'\s+','',s)
    return s

# build interp anchor pairs func47 -> func50 from confirmed
pairs=[]
for rva,c in conf.items():
    a=A.get(rva)
    if not a or a.get("func") is None: continue
    off=a["off"]; rva50=int(c["rva50"],16)
    pairs.append((a["func"], rva50-off))
pairs=sorted(set(pairs))
xs=[p[0] for p in pairs]
def interp(f47):
    i=bisect.bisect_left(xs,f47)
    if i<=0: return f47+(pairs[0][1]-pairs[0][0])
    if i>=len(pairs): return f47+(pairs[-1][1]-pairs[-1][0])
    lo=pairs[i-1]; hi=pairs[i]
    if hi[0]==lo[0]: return f47+(lo[1]-lo[0])
    t=(f47-lo[0])/(hi[0]-lo[0]); dlo=lo[1]-lo[0]; dhi=hi[1]-hi[0]
    return f47+int(dlo+t*(dhi-dlo))

# cache of normalized decompiled body per func-start(rva)
_cache={}
def body50(frva):
    if frva in _cache: return _cache[frva]
    try: b=norm(str(ida_hexrays.decompile(ib+frva)))
    except: b=""
    _cache[frva]=b; return b

WINDOW=0x8000; STEP=0x4
out={}; done=0
targets=[rva for rva in A if rva not in conf and A[rva].get("func") is not None]
for rva in targets:
    a=A[rva]; off=a["off"]; sz47=a["size"]; b47=B.get(rva,{}).get("norm","")
    if not b47: out[rva]={"status":"no-body"}; continue
    est=interp(a["func"])
    # collect candidate func-starts in window, prefilter by size
    cands={}
    ea=ib+est-WINDOW
    endw=ib+est+WINDOW
    f=ida_funcs.get_func(ea)
    cur=f.start_ea if f else ea
    while cur<endw:
        ff=ida_funcs.get_func(cur)
        if ff:
            s=ff.start_ea-ib
            if s not in cands:
                szd=abs((ff.end_ea-ff.start_ea)-sz47)
                if szd<=0x100: cands[s]=szd
            nxt=ff.end_ea
        else:
            nxt=idc.next_head(cur, endw)
            if nxt<=cur: nxt=cur+1
        cur=nxt
    # score candidates by body ratio (closest size first)
    best=(0.0,None); second=0.0
    for s in sorted(cands, key=lambda k:cands[k]):
        b=body50(s)
        if not b: continue
        r=difflib.SequenceMatcher(None,b47,b).ratio()
        if r>best[0]: second=best[0]; best=(r,s)
        elif r>second: second=r
    if best[1] is not None and best[0]>=0.90 and (best[0]-second)>=0.03:
        out[rva]={"status":"body-match","func50":hex(best[1]),"off":off,
                  "rva50":hex(best[1]+off),"ratio":round(best[0],3),"second":round(second,3)}
    else:
        out[rva]={"status":"no-body-match","best":round(best[0],3),
                  "bestfunc":hex(best[1]) if best[1] else None,"est":hex(est),"ncands":len(cands)}
    done+=1
    if done%20==0: print("...",done,"/",len(targets))
open(W+r"\bodymatch_e.json","w").write(json.dumps(out,ensure_ascii=True,indent=0))
from collections import Counter
print("DONE",dict(Counter(v["status"] for v in out.values())))
ida_pro.qexit(0)
