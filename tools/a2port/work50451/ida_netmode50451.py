import idaapi, idc, idautils, ida_funcs, ida_hexrays, ida_bytes, ida_pro, json
idaapi.auto_wait()
ib=idaapi.get_imagebase()
out={"imagebase":hex(ib)}

def fn(ea):
    f=ida_funcs.get_func(ea); return (hex(f.start_ea-ib), f.end_ea-f.start_ea) if f else (None,0)
def dc(ea,limit=1400):
    f=ida_funcs.get_func(ea)
    if not f: return None
    try: return str(ida_hexrays.decompile(f.start_ea))[:limit]
    except Exception as e: return "ERR %s"%e
def callees(ea):
    f=ida_funcs.get_func(ea)
    if not f: return []
    cs=[];e=f.start_ea
    while e<f.end_ea:
        if idc.print_insn_mnem(e)=="call":
            t=idc.get_operand_value(e,0)
            if t and ida_funcs.get_func(t): cs.append(hex(t-ib))
        e=idc.next_head(e,f.end_ea)
    return cs

# 1) Do the 47031 netmode RVAs port directly? decompile them in the 50451 IDB
out["direct_check"]={}
for name,rva in [("GetNetMode_47031",0x41F5A60),("WorldGetNetMode_47031",0x4040990),("A2GetNetMode_47031",0x54AF360)]:
    b=dc(ib+rva, 500)
    out["direct_check"][name]={"rva":hex(rva),"func":fn(ib+rva)[0],"size":fn(ib+rva)[1],"body":b}

# 2) PREPHYSBONES anchor -> exec handler -> first GetNetMode-shaped call
hits=[]
for s in idautils.Strings():
    t=str(s)
    if "PREPHYSBONES" in t.upper():
        hits.append((s.ea,t))
out["prephysbones_strings"]=[{"ea":hex(e),"s":t} for e,t in hits[:6]]
anchor=None
if hits:
    for sea,_ in hits:
        for x in idautils.XrefsTo(sea,0):
            f=ida_funcs.get_func(x.frm)
            if f: anchor=f.start_ea; break
        if anchor: break
if anchor:
    out["prephys_handler"]={"rva":hex(anchor-ib),"callees":callees(anchor)[:12],"body":dc(anchor,2000)}
with open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\netmode50451.json","w",encoding="utf-8") as f:
    json.dump(out,f,indent=1)
print("WROTE")
ida_pro.qexit(0)