import json,difflib
W="C:/Users/drych/ProjectDrift/Solara/tools/a2port/work50451/"
b47=json.load(open(W+"bodies47.json")); b50=json.load(open(W+"bodies50_2.json"))
M=json.load(open(W+"rvamap_final2.json"))
conf={}; rej={}
for rva,m in M.items():
    st=m.get("status")
    if st not in("matched","interp-verified"): continue
    n47=b47.get(rva,{}).get("norm",""); n50=b50.get(rva,{}).get("norm","")
    ratio = 1.0 if (n47 and n50 and n47==n50) else (difflib.SequenceMatcher(None,n47,n50).ratio() if (n47 and n50) else 0.0)
    rec={"rva50":m["rva50"],"status":st,"ratio":round(ratio,3)}
    if st=="matched":                 conf[rva]=rec           # string anchor = reliable, keep regardless of body drift
    elif ratio>=0.90:                 conf[rva]=rec           # interp vetted by body similarity
    else:                             rej[rva]=rec
json.dump({"confirmed":conf,"rejected":rej},open(W+"confirm2.json","w"),indent=0)
print("CONFIRMED=%d (string=%d interp=%d)  REJECTED(interp)=%d"%(
  len(conf), sum(1 for v in conf.values() if v["status"]=="matched"),
  sum(1 for v in conf.values() if v["status"]=="interp-verified"), len(rej)))
