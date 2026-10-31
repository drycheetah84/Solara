import json, os, re, sys
W=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
SRC=r"C:\Users\drych\ProjectDrift\Solara\Server"
# load resolved maps: 47031 RVA (upper hex, no 0x) -> 50451 RVA (hex str 0x..)
M={}
fin=json.load(open(os.path.join(W,"final_map.json")))
for rva,v in fin.items(): M[rva.upper()]=v["rva50"]
for extra in ["dataref_50451.json","extras_resolved.json"]:
    p=os.path.join(W,extra)
    if os.path.exists(p):
        for rva,v in json.load(open(p)).items():
            if v.get("rva50"): M[rva.upper()]=v["rva50"]
def norm50(h):
    return "0x%X"%int(h,16)
# live-code RVA patterns: capture the hex and rewrite only inside code (not // comments)
LIVE=re.compile(r'((?:GetBase\(\)|base|b0|\bb)\s*\+\s*0x0*|_RVA\s*=\s*0x0*|WriteByte\(\s*(?:GetBase\(\)|base)\s*\+\s*0x0*)([0-9A-Fa-f]{5,8})')
report={"applied":[],"unmapped":[],"gated":[]}
for fn in ["dllmain_50451.cpp","specedit.h"]:
    path=os.path.join(SRC,fn)
    lines=open(path,encoding="utf-8",errors="ignore").read().split("\n")
    changed=0
    for i,l in enumerate(lines):
        # split off trailing comment to protect it
        ci=l.find("//")
        code = l if ci<0 else l[:ci]
        tail = "" if ci<0 else l[ci:]
        def repl(m):
            global changed
            pre=m.group(1); h=m.group(2).upper()
            new=M.get(h)
            if new is None:
                report["unmapped"].append((fn,i+1,h)); return m.group(0)
            # rebuild prefix preserving 0x then new value (strip 0x from prefix's literal)
            prefix=re.sub(r'0x0*$','0x', pre)
            changed_local=norm50(new)[2:]
            report["applied"].append((fn,i+1,h,norm50(new)))
            return prefix+changed_local
        code2=LIVE.sub(repl, code)
        lines[i]=code2+tail
    open(path,"w",encoding="utf-8",newline="\n").write("\n".join(lines))
    print("%s: edits=%d"%(fn,changed))
json.dump(report, open(os.path.join(W,"apply_report.json"),"w"), indent=1)
print("APPLIED=%d UNMAPPED=%d"%(len(report["applied"]),len(report["unmapped"])))
seen=set()
for fn,ln,h in report["unmapped"]:
    if h in seen: continue
    seen.add(h); print("  UNMAPPED",h,"first@",fn,ln)
