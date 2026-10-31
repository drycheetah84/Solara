import idaapi, idc, ida_funcs, ida_hexrays, ida_pro, json, re
idaapi.auto_wait()
ib=idaapi.get_imagebase()
W=r"C:\Users\drych\ProjectDrift\Solara\tools\a2port\work50451"
def norm(s):
    if not s: return ""
    s=re.sub(r'sub_[0-9A-Fa-f]+','F',s)
    s=re.sub(r'(byte|word|dword|qword|unk|off|loc|stru|xmmword|flt|dbl)_[0-9A-Fa-f]+','G',s)
    s=re.sub(r'0x[0-9A-Fa-f]+','N',s); s=re.sub(r'\b[0-9]+\b','N',s)
    s=re.sub(r'\bv[0-9]+\b','v',s); s=re.sub(r'\ba[0-9]+\b','a',s)
    s=re.sub(r'\s+','',s); return s
EXTRA=[0xFCC220,0x54B13D0,0x54E3740]
out={}
for rva in EXTRA:
    ea=ib+rva; f=ida_funcs.get_func(ea)
    if not f:
        idc.create_insn(ea); ida_funcs.add_func(ea); f=ida_funcs.get_func(ea)
    rec={"func": (f.start_ea-ib) if f else None, "off": (ea-f.start_ea) if f else 0,
         "size": (f.end_ea-f.start_ea) if f else 0}
    try: rec["norm"]=norm(str(ida_hexrays.decompile(f.start_ea))) if f else ""
    except: rec["norm"]=""
    out["%X"%rva]=rec
# FName::ToString func bounds (start 0x12B8EF0) for end-offset of 0x12B9074
f=ida_funcs.get_func(ib+0x12B8EF0)
out["_fname"]={"start":hex(0x12B8EF0),"func": (f.start_ea-ib) if f else None,
               "end":(f.end_ea-ib) if f else None,"endoff":(0x12B9074-(f.start_ea-ib)) if f else None}
open(W+r"\extras_47031.json","w").write(json.dumps(out,ensure_ascii=True))
print("WROTE extras", {k:(v.get('func') if isinstance(v,dict) else v) for k,v in out.items()})
ida_pro.qexit(0)
