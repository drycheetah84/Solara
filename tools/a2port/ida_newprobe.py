import idautils, idc, idaapi, json
named=0; a2=0; samples=[]
for ea in idautils.Functions():
    n=idc.get_func_name(ea)
    if n and not n.startswith("sub_"):
        named+=1
        if ('A2' in n or 'VRPawn' in n or 'Ticket' in n or 'GameState' in n) and len(samples)<12:
            samples.append(n)
open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\newprobe.json","w").write(json.dumps({"total":sum(1 for _ in idautils.Functions()),"named":named,"samples":samples}))
