import idaapi, idc, idautils, ida_funcs, ida_bytes, ida_pro, json
idaapi.auto_wait()
ib=idaapi.get_imagebase()
W=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
A=json.load(open(W+r"\anchors_47031.json"))
globs=[rva for rva in A if A[rva].get("func") is None]

def datarefs_from(f):
    # ordered list of (ea, target_rva) data refs within func f
    o=[]; e=f.start_ea
    while e<f.end_ea:
        for dr in idautils.DataRefsFrom(e):
            o.append((e-ib, dr-ib))
        e=idc.next_head(e,f.end_ea)
    return o

out={}
for rva in globs:
    g=int(rva,16); ea=ib+g
    seg=idc.get_segm_name(ea)
    # is this inside a function (code site) even though get_func(ea) None? check containing func of nearest
    cf=ida_funcs.get_func(ea)
    info={"seg":seg,"in_func":(cf.start_ea-ib) if cf else None,
          "bytes":[idc.get_wide_byte(ea+i) for i in range(8)]}
    # collect referrers
    refs=[]
    for x in idautils.XrefsTo(ea,0):
        fr=x.frm; ff=ida_funcs.get_func(fr)
        if ff:
            drs=datarefs_from(ff)
            # ordinal of this ref within func data refs to this target
            idx=None
            for i,(sea,tgt) in enumerate(drs):
                if sea==(fr-ib) and tgt==g: idx=i; break
            refs.append({"func":ff.start_ea-ib,"site":fr-ib,"mnem":idc.print_insn_mnem(fr),
                         "ndrefs":len(drs),"ordinal":idx})
    info["refs"]=refs[:12]; info["nrefs"]=len(refs)
    out[rva]=info
open(W+r"\dataref_47031.json","w").write(json.dumps(out,ensure_ascii=True,indent=0))
print("WROTE dataref for",len(out),"globals; with-refs=",sum(1 for v in out.values() if v["nrefs"]>0))
ida_pro.qexit(0)
