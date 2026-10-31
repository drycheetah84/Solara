import idaapi, idc, ida_funcs, ida_pro, json, bisect
idaapi.auto_wait()
ib=idaapi.get_imagebase()
A=json.load(open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\anchors_47031.json"))
M=json.load(open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\rvamap_50451.json"))
# anchor pairs func47 -> func50 (from string matches)
_cf=json.load(open(r"C:/Users/drych/ProjectDrift/Solara/tools/a2port/work50451/confirm.json"))["confirmed"]
pairs=sorted(set((A[r]["func"], int(c["rva50"],16)-A[r]["off"]) for r,c in _cf.items() if A.get(r,{}).get("func") is not None))
xs=[p[0] for p in pairs]
def interp_delta(f47):
    i=bisect.bisect_left(xs,f47)
    if i<=0: return pairs[0][1]-pairs[0][0]
    if i>=len(pairs): return pairs[-1][1]-pairs[-1][0]
    lo=pairs[i-1]; hi=pairs[i]
    # linear interpolation of delta between nearest anchors
    if hi[0]==lo[0]: return lo[1]-lo[0]
    t=(f47-lo[0])/(hi[0]-lo[0]); dlo=lo[1]-lo[0]; dhi=hi[1]-hi[0]
    return int(dlo + t*(dhi-dlo))
def ncallees(f):
    cs=0;e=f.start_ea
    while e<f.end_ea:
        if idc.print_insn_mnem(e)=="call" and ida_funcs.get_func(idc.get_operand_value(e,0)): cs+=1
        e=idc.next_head(e,f.end_ea)
    return cs
out={}; acc=0
for rva,a in A.items():
    m=M.get(rva,{})
    if m.get("status")=="matched":
        out[rva]={"status":"matched","rva50":m["rva50"],"func50":m["func50"],"off":a["off"]}; acc+=1; continue
    if a.get("func") is None:
        out[rva]={"status":"no-func-47031"}; continue
    f47=a["func"]; off=a["off"]; sz47=a["size"]; nc47=len(a.get("callees") or [])
    exp=f47+interp_delta(f47)
    best=None
    # scan a small window around the interpolated func start for a function whose size & callee-count match
    for cand in range(exp-0x400, exp+0x400, 0x10):
        f=ida_funcs.get_func(ib+cand)
        if not f: continue
        s=f.start_ea-ib
        if best is not None and best==s: continue
        szd=abs((f.end_ea-f.start_ea)-sz47)
        if szd<=0x30:
            ncd=abs(ncallees(f)-nc47)
            if ncd<=2:
                out[rva]={"status":"interp-verified","func50":hex(s),"off":off,"rva50":hex(s+off),
                          "szdelta":szd,"ncdelta":ncd,"interp":hex(exp)}; acc+=1; best=s; break
    if rva not in out:
        out[rva]={"status":"unresolved","func47":hex(f47),"off":off,"interp":hex(exp),"sz47":sz47,"nc47":nc47}
open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\rvamap_final2.json","w").write(json.dumps(out,ensure_ascii=True,indent=0))
from collections import Counter
print("resolved=%d/%d  status=%s"%(acc,len(out),dict(Counter(v["status"] for v in out.values()))))
ida_pro.qexit(0)