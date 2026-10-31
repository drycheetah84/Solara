import idaapi,idc,ida_funcs,ida_hexrays,ida_pro,json
idaapi.auto_wait()
ib=idaapi.get_imagebase()
out={"input":idaapi.get_input_file_path(),"imagebase":hex(ib),"func_count":0}
# count functions
import ida_funcs
out["func_count"]=ida_funcs.get_func_qty()
# sanity: ProcessEvent (Basic.hpp 0x14E1B10) should be a function
for name,rva in [("ProcessEvent",0x14E1B10)]:
    f=ida_funcs.get_func(ib+rva)
    out[name]={"rva":hex(rva),"ea":hex(ib+rva),"isfunc":bool(f),"start":hex(f.start_ea) if f else None}
open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\verify.json","w").write(json.dumps(out,indent=1))
print("OK"); ida_pro.qexit(0)
