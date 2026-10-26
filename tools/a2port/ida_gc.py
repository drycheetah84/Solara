# NAMED 47031 IDB. (1) decompile the crash site + caller to confirm GC/cluster
# code. (2) find the gc.AllowParallelGC / gc.CreateGCClusters cvar backing-int
# addresses: locate the cvar-name strings, the functions that reference them
# (RegisterConsoleVariableRef binds &backingInt), and dump the data globals those
# functions touch. Read-only.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_hexrays

A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG = open(A + r"\gc_log.txt", "w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()

try:
    ida_hexrays.init_hexrays_plugin()
    IB = idaapi.get_imagebase()
    log("imagebase", hex(IB))
    def rva(ea): return ea-IB
    def va(r): return IB+r

    def decomp(r, cap=2600):
        try:
            cf = ida_hexrays.decompile(va(r))
            return str(cf)[:cap] if cf else None
        except Exception as e:
            return "ERR:"+str(e)

    def data_globals(r):
        out=[]
        f=ida_funcs.get_func(va(r))
        if not f: return out
        for item in idautils.FuncItems(f.start_ea):
            for d in idautils.DataRefsFrom(item):
                if ida_funcs.get_func(d): continue
                out.append({"rva":hex(rva(d)),"size":idc.get_item_size(d),
                            "name":idc.get_name(d),"at":hex(rva(item)),
                            "disasm":idc.generate_disasm_line(item,0)})
        # de-dup by rva
        seen,res=set(),[]
        for o in out:
            if o["rva"] not in seen: seen.add(o["rva"]); res.append(o)
        return res

    out={"crash":{}, "cvars":{}}
    for r in (0x4E3A31A, 0x4E3CFB0, 0x2E5F4E4, 0x111FE4F):
        f=ida_funcs.get_func(va(r))
        out["crash"][hex(r)]={"func":idc.get_func_name(f.start_ea) if f else None,
                              "func_rva":hex(rva(f.start_ea)) if f else None,
                              "decomp":decomp(rva(f.start_ea) if f else r)}
        log("crash", hex(r), out["crash"][hex(r)]["func"])

    # cvar strings -> referencing funcs -> data globals (backing ints)
    targets=["gc.AllowParallelGC","gc.CreateGCClusters"]
    for st in idautils.Strings():
        try: t=str(st)
        except Exception: continue
        if t in targets:
            funcs=set()
            for x in idautils.XrefsTo(st.ea):
                f=ida_funcs.get_func(x.frm)
                if f: funcs.add(f.start_ea)
            recs=[]
            for fea in funcs:
                recs.append({"func_rva":hex(rva(fea)),"func":idc.get_func_name(fea),
                             "int_globals":[g for g in data_globals(rva(fea)) if g["size"] in (4,1)][:20],
                             "decomp":decomp(rva(fea),2000)})
            out["cvars"][t]=recs
            log("cvar", t, "funcs", [hex(rva(f)) for f in funcs])

    json.dump(out, open(A+r"\gc-dump.json","w"), indent=1, default=str)
    open(A+r"\gc_ok.txt","w").write("ok")
    log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc())
    open(A+r"\gc_err.txt","w").write(traceback.format_exc())
finally:
    LOG.close()
