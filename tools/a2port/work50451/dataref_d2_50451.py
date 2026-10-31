import idaapi, idc, idautils, ida_funcs, ida_bytes, ida_pro, json, os
idaapi.auto_wait()
ib=idaapi.get_imagebase()
W=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
D=json.load(open(os.path.join(W,"dataref_47031.json")))
FM=json.load(open(os.path.join(W,"funcmap.json")))   # func47hex -> func50hex

def datarefs_from(fstart):
    f=ida_funcs.get_func(fstart)
    if not f: return []
    o=[]; e=f.start_ea
    while e<f.end_ea:
        for dr in idautils.DataRefsFrom(e):
            o.append(dr-ib)
        e=idc.next_head(e,f.end_ea)
    return o

def segof(rva):
    return idc.get_segm_name(ib+rva)

out={}
for rva,info in D.items():
    g=int(rva,16)
    # Case A: in-func code site (byte-patch) -> map containing func + byte pattern
    if info.get("in_func") is not None:
        f47=info["in_func"]; f50h=FM.get("%X"%f47)
        if f50h:
            f50=int(f50h,16); off=g-f47
            sig=bytes(info.get("bytes") or [])
            # try same relative offset first
            cand=f50+off
            curbytes=bytes(idc.get_wide_byte(ib+cand+i) for i in range(len(sig)))
            if curbytes==sig:
                out[rva]={"status":"code-site-reloff","rva50":hex(cand),"func50":f50h}; continue
            # else search the sig within func50
            f=ida_funcs.get_func(ib+f50)
            if f and sig:
                hit=None; e=f.start_ea
                while e<f.end_ea-len(sig):
                    if bytes(idc.get_wide_byte(e+i) for i in range(len(sig)))==sig: hit=e-ib; break
                    e+=1
                if hit is not None:
                    out[rva]={"status":"code-site-bytesearch","rva50":hex(hit),"func50":f50h}; continue
            out[rva]={"status":"code-site-unresolved","func50":f50h,"reloff_cand":hex(cand)}; continue
        out[rva]={"status":"code-site-no-funcmap","func47":hex(f47)}; continue
    # Case B: data global -> vote via referrer funcs + ordinal
    votes={}
    for r in info.get("refs",[]):
        f47=r["func"]; f50h=FM.get("%X"%f47); ordn=r.get("ordinal")
        if f50h is None or ordn is None: continue
        drs=datarefs_from(ib+int(f50h,16))
        if ordn < len(drs):
            cand=drs[ordn]
            # only count data-segment candidates
            sg=segof(cand)
            if sg in (".data",".rdata",".bss",".pdata") or cand>0x7000000:
                votes[cand]=votes.get(cand,0)+1
    if votes:
        best=max(votes, key=lambda k:votes[k])
        out[rva]={"status":"data-vote","rva50":hex(best),"votes":votes[best],
                  "nvote":len(votes),"seg":segof(best)}
    else:
        out[rva]={"status":"data-unresolved","nrefs":info.get("nrefs"),"seg":info.get("seg")}
open(os.path.join(W,"dataref_50451.json"),"w").write(json.dumps(out,ensure_ascii=True,indent=0))
from collections import Counter
print("DATA D2:",dict(Counter(v["status"] for v in out.values())))
ida_pro.qexit(0)
