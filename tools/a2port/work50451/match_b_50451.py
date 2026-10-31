import idaapi, idc, idautils, ida_funcs, ida_pro, json
idaapi.auto_wait()
ib=idaapi.get_imagebase()
A=json.load(open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\anchors_47031.json"))
# 1) string -> set(func_rva50) index
sidx={}
for s in idautils.Strings():
    try: val=str(s)
    except: continue
    if not (4<=len(val)<=160): continue
    fs=set()
    for x in idautils.XrefsTo(s.ea,0):
        f=ida_funcs.get_func(x.frm)
        if f: fs.add(f.start_ea-ib)
    if fs:
        d=sidx.get(val)
        if d is None: sidx[val]=fs
        else: d|=fs
# 2) match each 47031 target by shared strings (intersection)
out={}; matched=0; amb=0; nomatch=0; nostr=0
for rva,a in A.items():
    strs=a.get("strings") or []
    if not strs:
        out[rva]={"status":"no-strings","func47":a.get("func"),"off":a.get("off"),"ncallees":len(a.get("callees") or [])}; nostr+=1; continue
    cand=None
    for s in strs:
        fs=sidx.get(s)
        if fs is None: continue
        cand = set(fs) if cand is None else (cand & fs)
        if cand is not None and len(cand)==0: break
    if cand and len(cand)==1:
        f50=next(iter(cand)); out[rva]={"status":"matched","func50":hex(f50),"off":a["off"],"rva50":hex(f50+a["off"])}; matched+=1
    elif cand and len(cand)>1:
        out[rva]={"status":"ambiguous","n":len(cand),"cands":[hex(c) for c in list(cand)[:6]],"off":a["off"]}; amb+=1
    else:
        out[rva]={"status":"no-match","strings":strs[:4]}; nomatch+=1
open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\rvamap_50451.json","w").write(json.dumps(out,ensure_ascii=True,indent=0))
print("matched=%d ambiguous=%d no-match=%d no-strings=%d total=%d"%(matched,amb,nomatch,nostr,len(out)))
ida_pro.qexit(0)