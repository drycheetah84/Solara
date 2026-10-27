# NAMED 47031 IDB. Find FCurlHttpRequest's vtable via MSVC RTTI, dump its slots
# with decompiled heads so ProcessRequest / SetURL / GetURL can be identified.
# Read-only.
import json, traceback
import idaapi, idautils, idc, ida_funcs, ida_bytes, ida_hexrays
A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG=open(A+r"\curlvt_log.txt","w")
def log(*a): LOG.write(" ".join(str(x) for x in a)+"\n"); LOG.flush()
try:
    ida_hexrays.init_hexrays_plugin(); IB=idaapi.get_imagebase()
    def rva(ea): return ea-IB
    # 1. RTTI type descriptor name ".?AVFCurlHttpRequest@@"
    name=".?AVFCurlHttpRequest@@"
    td=None
    for s in idautils.Strings():
        try: t=str(s)
        except Exception: continue
        if name in t: td=s.ea; break
    log("type descriptor name ea", hex(td) if td else None)
    out={"imagebase":hex(IB),"td":hex(rva(td)) if td else None,"vtables":[]}
    def decomp(ea,cap=600):
        try: cf=ida_hexrays.decompile(ea); return str(cf)[:cap] if cf else None
        except Exception as e: return "ERR:"+str(e)
    if td:
        # type descriptor starts 0x10 before the name (vftable ptr + spare + name)
        tdesc = td - 0x10
        # find Complete Object Locators referencing tdesc (COL has +0xC = RVA of tdesc)
        # then vtable = address right after a pointer to the COL.
        # Scan .rdata for 4-byte == rva(tdesc) (COL stores type descriptor as image-rel RVA)
        tdesc_rva = rva(tdesc)
        import struct
        for seg in idautils.Segments():
            sname=idaapi.get_segm_name(idaapi.getseg(seg))
            if sname not in (".rdata",): continue
            s=idaapi.getseg(seg)
            ea=s.start_ea
            data=ida_bytes.get_bytes(s.start_ea, s.end_ea-s.start_ea) or b""
            # find COLs: a 4-byte field == tdesc_rva at COL+0xC
            for off in range(0, len(data)-4, 4):
                val=int.from_bytes(data[off:off+4],"little")
                if val==tdesc_rva:
                    col_ea = s.start_ea+off-0xC   # tdesc rva is at COL+0xC
                    # find a pointer to col_ea (the vtable[-1] = COL ptr), vtable starts at +8
                    for xr in idautils.XrefsTo(col_ea):
                        vt = xr.frm + 8
                        slots=[]
                        for i in range(16):
                            p=ida_bytes.get_qword(vt+i*8)
                            f=ida_funcs.get_func(p)
                            if not f: break
                            slots.append({"i":i,"rva":hex(rva(p)),"name":idc.get_func_name(p),
                                          "decomp":decomp(p,400)})
                        if slots:
                            out["vtables"].append({"vtable_rva":hex(rva(vt)),"col_rva":hex(rva(col_ea)),"slots":slots})
                            log("vtable at", hex(rva(vt)), "slots", len(slots))
    json.dump(out,open(A+r"\curlvt-dump.json","w"),indent=1,default=str)
    open(A+r"\curlvt_ok.txt","w").write("ok")
    log("DONE")
except Exception:
    log("FATAL\n"+traceback.format_exc()); open(A+r"\curlvt_err.txt","w").write(traceback.format_exc())
finally: LOG.close()
