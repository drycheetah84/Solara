import json, traceback, idaapi, idautils, idc, ida_funcs, ida_hexrays
A=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def dec(rva,cap=7000):
        f=ida_funcs.get_func(IB+rva)
        try: return str(ida_hexrays.decompile(f.start_ea if f else IB+rva))[:cap]
        except Exception as e: return "ERR:"+str(e)
    out={"sub_144A3B4C0":dec(0x4a3b4c0),"sub_144A9C0D0":dec(0x4a9c0d0,3000)}
    json.dump(out,open(A+r"\a2level-dump.json","w"),indent=1,default=str)
    open(A+r"\a2level_ok.txt","w").write("ok")
except Exception:
    open(A+r"\a2level_err.txt","w").write(traceback.format_exc())
