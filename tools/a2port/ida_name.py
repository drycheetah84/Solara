# Name the NEW (47031) IDB from the Dumper-7 oracles so that structural /
# callee / thunk matching has anchors. Writes + saves the IDB.
#   - FunctionsInfo.json (14565 native UFunction name -> RVA)  => each exec/impl
#   - also label the known-verified engine globals for readability
# Idempotent: re-running just re-applies the same names.
import json, traceback
import idaapi, idc, ida_name, ida_funcs

A = r"C:\Users\drych\ProjectDrift\Solara\tools\a2port"
LOG = open(A + r"\name_log.txt", "w")


def log(*a):
    LOG.write(" ".join(str(x) for x in a) + "\n")
    LOG.flush()


try:
    IB = idaapi.get_imagebase()
    log("imagebase", hex(IB))
    name2rva = json.load(open(A + r"\funcinfo-name2rva.json"))
    log("funcinfo entries", len(name2rva))

    FL = ida_name.SN_NOCHECK | ida_name.SN_FORCE
    applied = skipped = already = 0
    for nm, rva in name2rva.items():
        ea = IB + int(rva)
        cur = idc.get_name(ea)
        if cur == nm:
            already += 1
            continue
        # only name code addresses that resolve to a function head (or create)
        f = ida_funcs.get_func(ea)
        if not f:
            # try to make it a function first
            ida_funcs.add_func(ea)
            f = ida_funcs.get_func(ea)
        tgt = f.start_ea if f else ea
        # prefer naming the exact ea; if it is mid-function, name the head too
        if ida_name.set_name(ea, nm, FL):
            applied += 1
        else:
            skipped += 1
        if applied and applied % 2000 == 0:
            log("applied", applied)
    log("done names applied=%d already=%d skipped=%d" % (applied, already, skipped))

    # readability labels for verified engine globals (do not affect porting)
    for gname, grva in (("g_GObjects", 0xA7C3A00), ("g_GNames", 0xA6DFBC0),
                        ("g_GWorld", 0xA9E0968)):
        try:
            ida_name.set_name(IB + grva, gname, FL)
        except Exception:
            pass

    log("saving database...")
    idc.save_database("")
    log("SAVED")
    open(A + r"\name_ok.txt", "w").write("applied=%d already=%d skipped=%d" % (applied, already, skipped))
except Exception:
    log("FATAL\n" + traceback.format_exc())
    open(A + r"\name_err.txt", "w").write(traceback.format_exc())
finally:
    LOG.close()
