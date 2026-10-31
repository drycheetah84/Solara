import idaapi, idc, idautils, ida_funcs, ida_bytes, ida_pro, json
idaapi.auto_wait()
ib=idaapi.get_imagebase()
targets=[int(l.strip(),16) for l in open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\rva_targets.txt") if l.strip()]
def strings_in(f):
    out=set(); e=f.start_ea
    while e<f.end_ea:
        for dr in idautils.DataRefsFrom(e):
            s=idc.get_strlit_contents(dr, -1, idc.STRTYPE_C)
            if s and 4<=len(s)<=80:
                try: out.add(s.decode("utf-8","ignore"))
                except: pass
            else:
                s2=idc.get_strlit_contents(dr,-1,idc.STRTYPE_C_16)
                if s2 and 4<=len(s2)<=160:
                    try: out.add(s2.decode("utf-16le","ignore"))
                    except: pass
        e=idc.next_head(e,f.end_ea)
    return sorted(out)
def callees_in(f):
    cs=[]; e=f.start_ea
    while e<f.end_ea:
        if idc.print_insn_mnem(e)=="call":
            t=idc.get_operand_value(e,0)
            tf=ida_funcs.get_func(t) if t else None
            if tf: cs.append(tf.start_ea-ib)
        e=idc.next_head(e,f.end_ea)
    # unique, ordered
    seen=set(); o=[]
    for c in cs:
        if c not in seen: seen.add(c); o.append(c)
    return o
res={}
for rva in targets:
    ea=ib+rva; f=ida_funcs.get_func(ea)
    if not f:
        res["%X"%rva]={"func":None}; continue
    res["%X"%rva]={"func":f.start_ea-ib,"off":ea-f.start_ea,"size":f.end_ea-f.start_ea,
                   "strings":strings_in(f),"callees":callees_in(f)[:40]}
open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\anchors_47031.json","w",encoding="utf-8").write(json.dumps(res))
print("WROTE anchors for",len(res),"targets")
ida_pro.qexit(0)