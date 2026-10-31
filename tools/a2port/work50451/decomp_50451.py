import idaapi, idc, ida_funcs, ida_hexrays, ida_pro, json, re
idaapi.auto_wait()
ib=idaapi.get_imagebase()
M=json.load(open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\rvamap_final2.json"))
def norm(s):
    if not s: return ""
    s=re.sub(r'sub_[0-9A-Fa-f]+','F',s)
    s=re.sub(r'(byte|word|dword|qword|unk|off|loc|stru|xmmword|flt|dbl)_[0-9A-Fa-f]+','G',s)
    s=re.sub(r'0x[0-9A-Fa-f]+','N',s); s=re.sub(r'\b[0-9]+\b','N',s)
    s=re.sub(r'\bv[0-9]+\b','v',s); s=re.sub(r'\ba[0-9]+\b','a',s)
    s=re.sub(r'\s+','',s); return s
out={}
for rva,m in M.items():
    if m.get("status") not in("matched","interp-verified"): continue
    f50=int(m["func50"],16); f=ida_funcs.get_func(ib+f50)
    if not f: continue
    try: body=str(ida_hexrays.decompile(f.start_ea))
    except: body=""
    out[rva]={"norm":norm(body),"len":len(body)}
open(r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451\bodies50_2.json","w").write(json.dumps(out,ensure_ascii=True))
print("WROTE 50451 candidate bodies",len(out))
ida_pro.qexit(0)
