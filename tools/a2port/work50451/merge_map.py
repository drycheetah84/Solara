import json, os
W=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
A=json.load(open(os.path.join(W,"anchors_47031.json")))
C=json.load(open(os.path.join(W,"confirm2.json")))["confirmed"]
E=json.load(open(os.path.join(W,"bodymatch_e.json"))) if os.path.exists(os.path.join(W,"bodymatch_e.json")) else {}
# sharded resumable body-search (bodysearch_f.py): one json record per line per shard
import glob
for p in sorted(glob.glob(os.path.join(W,"bodymatch_f_*.jsonl"))):
    for ln in open(p):
        try: r=json.loads(ln)
        except Exception: continue
        E[r["rva47"]]=r

# authoritative per-target rva50 + provenance
final={}
# manual confirmations
MAN={"41F5A60":"0x41f5a20"}  # GetNetMode
for rva,v in MAN.items(): final[rva]={"rva50":v,"src":"manual"}
for rva,c in C.items():
    final[rva]={"rva50":c["rva50"],"src":"confirm2:"+c.get("status","?"),"ratio":c.get("ratio")}
for rva,v in E.items():
    if v.get("status")=="body-match" and rva not in final:
        final[rva]={"rva50":v["rva50"],"src":"bodysearch","ratio":v.get("ratio")}

# func47 -> func50 map (rva-start hex keys), from anchors off
funcmap={}
for rva,f in final.items():
    a=A.get(rva)
    if not a or a.get("func") is None: continue
    off=a["off"]; r50=int(f["rva50"],16)
    f47=a["func"]; f50=r50-off
    funcmap["%X"%f47]="%X"%f50

json.dump(funcmap, open(os.path.join(W,"funcmap.json"),"w"))
json.dump(final, open(os.path.join(W,"final_map.json"),"w"), indent=0)
nfunc=sum(1 for rva in A if A[rva].get("func") is not None)
print("final confirmed targets=%d (of %d func + %d data)"%(len(final), nfunc, len(A)-nfunc))
print("funcmap pairs=%d"%len(funcmap))
