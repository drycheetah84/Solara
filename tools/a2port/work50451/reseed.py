p="C:/Users/drych/ProjectDrift/Solara/tools/a2port/work50451/resolve_c_50451.py"
s=open(p,encoding="utf-8").read()
old='pairs=sorted((A[r]["func"], int(m["func50"],16)) for r,m in M.items() if m["status"]=="matched" and A[r].get("func") is not None)'
new=('_cf=json.load(open(r"C:/Users/drych/ProjectDrift/Solara/tools/a2port/work50451/confirm.json"))["confirmed"]\n'
     'pairs=sorted(set((A[r]["func"], int(c["rva50"],16)-A[r]["off"]) for r,c in _cf.items() if A.get(r,{}).get("func") is not None))')
if old in s:
    s=s.replace(old,new).replace('rvamap_final.json','rvamap_final2.json')
    open(p,"w",encoding="utf-8").write(s); print("reseeded OK; output -> rvamap_final2.json")
else: print("WARN: anchor line not found (maybe already reseeded?)")
