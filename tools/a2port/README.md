# A2 DLL offset porting (tools/a2port)

Automated first pass of porting the HalcyonA2 injected-DLL offsets to a new A2 build.
dllmain.cpp (authoritative tree: OneDrive\...\rigel-server\HalcyonA2\HalcyonA2) holds the
build-specific addresses directly as `static constexpr ... _RVA = 0x...` + inline `base + 0x...`.

- `extract_sites.py`  pull every address out of dllmain.cpp, bucket into rva / struct / bound
- `port_offsets.py`   (copied from the real tree) capstone signature match OLD.exe -> NEW.exe
- `worksheet.py`      join sites + porter result + new-SDK globals -> worksheet.json
- `gen_report.py`     -> PORT-47031.md (the human worksheet)

Inputs for 22284 -> 47031:
  OLD = OneDrive\...\rigel-server\SpecEditorBuild\...\A2-Win64-Shipping.exe  (22284, matches dllmain)
  NEW = Downloads\32586841537580874\...\A2-Win64-Shipping.exe               (47031)
  SDK = C:\Dumper-7\5.5.3-47031+++A2+hotfix-A2\CppSDK\SDK

Re-run:  ./.venv/Scripts/python.exe extract_sites.py "<dllmain.cpp>" -o sites.json
         ./.venv/Scripts/python.exe port_offsets.py "<OLD.exe>" "<NEW.exe>" sites.porter.json -o port-result.json
         ./.venv/Scripts/python.exe worksheet.py && ./.venv/Scripts/python.exe gen_report.py

STATUS: candidates are UNVERIFIED. See PORT-47031.md section 6. Not baked into dllmain.cpp.
