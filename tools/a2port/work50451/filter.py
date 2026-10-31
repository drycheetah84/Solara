vals=[l.strip() for l in open("C:/Users/drych/ProjectDrift/Solara/tools/a2port/work50451/rva_list.txt") if l.strip()]
keep=[]
for v in vals:
    try: n=int(v,16)
    except: continue
    if 0x1000000<=n<=0xC000000 and (n & 0xFFFF)!=0:
        keep.append(n)
keep=sorted(set(keep))
open("C:/Users/drych/ProjectDrift/Solara/tools/a2port/work50451/rva_targets.txt","w").write("\n".join(hex(n) for n in keep))
print("filtered RVA targets:",len(keep))
