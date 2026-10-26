import json, idaapi, idautils, idc, ida_funcs, ida_hexrays, ida_nalt
out = {"imagebase": idaapi.get_imagebase(), "input": ida_nalt.get_root_filename(),
       "func_count": sum(1 for _ in idautils.Functions()), "hexrays": False}
try:
    out["hexrays"] = bool(ida_hexrays.init_hexrays_plugin())
except Exception as e:
    out["hexrays_err"] = str(e)
# sample: resolve one known A2 site by RVA
ib = out["imagebase"]
ea = ib + 0x544D9B0  # JwtBuild on 22284
f = ida_funcs.get_func(ea)
if f:
    out["sample_JwtBuild"] = {"name": idc.get_func_name(f.start_ea),
                              "start_rva": hex(f.start_ea - ib), "size": f.end_ea - f.start_ea}
open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\probe.json","w").write(json.dumps(out, indent=1))
