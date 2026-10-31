import idaapi, idc, idautils, ida_funcs, ida_bytes, ida_hexrays, ida_pro, json
idaapi.auto_wait()
ib=idaapi.get_imagebase()
GN=ib+0x41F5A20   # GetNetMode (InternalGetNetMode switch fn)
out={"imagebase":hex(ib),"getnetmode":hex(0x41F5A20)}

def func_rva(ea):
    f=ida_funcs.get_func(ea); return (f.start_ea-ib) if f else None
def fsize(ea):
    f=ida_funcs.get_func(ea); return (f.end_ea-f.start_ea) if f else 0

# 1) direct callers of GetNetMode
callers=set()
for x in idautils.CodeRefsTo(GN,0):
    f=ida_funcs.get_func(x)
    if f: callers.add((f.start_ea, x))
out["num_callers"]=len(callers)

# 2) thin-wrapper callers (small fns that call GN) = WorldGetNetMode / A2GetNetMode candidates
wrappers=[]
for fs,_ in callers:
    sz=fsize(fs)
    if sz and sz<0x80: wrappers.append({"rva":hex(fs-ib),"size":sz})
out["thin_wrappers"]=sorted(wrappers,key=lambda w:w["size"])[:20]

# 3) netmode==2 gate sites: within each caller, a `cmp <reg>,2` shortly AFTER a call to GN
gates=[]
for fs,callsite in callers:
    f=ida_funcs.get_func(fs)
    if not f: continue
    e=f.start_ea
    while e<f.end_ea:
        mn=idc.print_insn_mnem(e)
        if mn=="cmp":
            # cmp reg, imm  where imm==2 ; the imm byte is the patch target
            if idc.get_operand_type(e,1)==5 and idc.get_operand_value(e,1)==2:
                # record the instruction + the location of the imm byte (last byte for a small imm form)
                sz=idc.get_item_size(e)
                gates.append({"ins_rva":hex(e-ib),"func":hex(fs-ib),"disasm":idc.generate_disasm_line(e,0),
                              "immbyte_rva":hex(e-ib+sz-1),"module":"A2" if (e-ib)>=0x5500000 else "engine"})
        e=idc.next_head(e,f.end_ea)
out["num_gates"]=len(gates)
from collections import Counter
out["gates_by_module"]=dict(Counter(g["module"] for g in gates))
out["gates"]=gates[:120]
with open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\nm2_50451.json","w",encoding="utf-8") as f:
    json.dump(out,f,indent=1)
print("WROTE callers=%d gates=%d"%(len(callers),len(gates)))
ida_pro.qexit(0)