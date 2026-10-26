// a2inject — inject the HalcyonA2 server DLL into the running A2 client and
// watch its log. LoadLibrary injection (the DLL spawns its own boot thread on
// DLL_PROCESS_ATTACH). x64 only; run from an x64 toolchain (go build default).
//
//	go build -o bin/a2inject.exe ./cmd/a2inject
//	a2inject.exe                 # auto-find A2-Win64-Shipping.exe, inject default DLL, tail log
//	a2inject.exe -dll C:\path\HalcyonA2.dll -proc A2-Win64-Shipping.exe
//	a2inject.exe -pid 1234 -watch=false
//
// Needs the same elevation as the target (run the shell as admin if the game is).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	pLoadLibraryW    = kernel32.NewProc("LoadLibraryW")
	pVirtualAllocEx  = kernel32.NewProc("VirtualAllocEx")
	pWriteProcessMem = kernel32.NewProc("WriteProcessMemory")
	pCreateRemoteThd = kernel32.NewProc("CreateRemoteThread")
	pGetExitCodeThd  = kernel32.NewProc("GetExitCodeThread")
)

const (
	memCommitReserve = 0x1000 | 0x2000
	pageReadWrite    = 0x04
	procAllAccess    = windows.PROCESS_CREATE_THREAD | windows.PROCESS_QUERY_INFORMATION |
		windows.PROCESS_VM_OPERATION | windows.PROCESS_VM_WRITE | windows.PROCESS_VM_READ
)

func findPID(name string) (uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), name) {
			return e.ProcessID, nil
		}
	}
	return 0, fmt.Errorf("process %q not found (is the game running?)", name)
}

func inject(pid uint32, dll string) error {
	abs, err := filepath.Abs(dll)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("DLL not found: %s", abs)
	}
	h, err := windows.OpenProcess(procAllAccess, false, pid)
	if err != nil {
		return fmt.Errorf("OpenProcess(pid=%d): %w (try running this as Administrator)", pid, err)
	}
	defer windows.CloseHandle(h)

	w, err := windows.UTF16FromString(abs)
	if err != nil {
		return err
	}
	size := uintptr(len(w) * 2)
	remote, _, e1 := pVirtualAllocEx.Call(uintptr(h), 0, size, memCommitReserve, pageReadWrite)
	if remote == 0 {
		return fmt.Errorf("VirtualAllocEx: %v", e1)
	}
	var wrote uintptr
	r, _, e2 := pWriteProcessMem.Call(uintptr(h), remote, uintptr(unsafe.Pointer(&w[0])), size, uintptr(unsafe.Pointer(&wrote)))
	if r == 0 {
		return fmt.Errorf("WriteProcessMemory: %v", e2)
	}
	load := pLoadLibraryW.Addr()
	th, _, e3 := pCreateRemoteThd.Call(uintptr(h), 0, 0, load, remote, 0, 0)
	if th == 0 {
		return fmt.Errorf("CreateRemoteThread(LoadLibraryW): %v", e3)
	}
	hThread := windows.Handle(th)
	defer windows.CloseHandle(hThread)
	windows.WaitForSingleObject(hThread, 15000)
	var exit uint32
	pGetExitCodeThd.Call(uintptr(hThread), uintptr(unsafe.Pointer(&exit))) // LoadLibraryW HMODULE truncated to 32-bit; 0 == load failed
	if exit == 0 {
		return fmt.Errorf("LoadLibraryW in target returned NULL — the DLL failed to load (missing dep / arch mismatch / DllMain threw)")
	}
	fmt.Printf("[a2inject] injected %s into pid %d (remote module low32=0x%X)\n", filepath.Base(abs), pid, exit)
	return nil
}

// tailLog follows the DLL's %TEMP%\HalcyonA2.log, flagging the server-flip and crashes.
func tailLog(path string, from int64) {
	fmt.Printf("[a2inject] tailing %s (Ctrl+C to stop)\n", path)
	off := from
	flipped, crashed := false, false
	for {
		f, err := os.Open(path)
		if err != nil {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if _, err := f.Seek(off, 0); err == nil {
			buf := make([]byte, 64*1024)
			for {
				n, _ := f.Read(buf)
				if n == 0 {
					break
				}
				off += int64(n)
				s := string(buf[:n])
				os.Stdout.WriteString(s)
				low := strings.ToLower(s)
				if !flipped && (strings.Contains(low, "gizserver") || strings.Contains(low, "gisserver") ||
					strings.Contains(low, "server mode") || strings.Contains(low, "listen")) {
					flipped = true
					fmt.Println("\n[a2inject] >>> server-mode indicator seen in log")
				}
				if !crashed && (strings.Contains(s, "[CRASH]") || strings.Contains(s, "FATAL") ||
					strings.Contains(low, "exception")) {
					crashed = true
					fmt.Println("\n[a2inject] !!! crash/fatal marker seen — capture this section")
				}
			}
		}
		f.Close()
		time.Sleep(300 * time.Millisecond)
	}
}

func main() {
	def := `C:\Users\drych\ProjectDrift\Solara\build\47031\x64\Release\HalcyonA2.dll`
	dll := flag.String("dll", def, "path to HalcyonA2.dll")
	proc := flag.String("proc", "A2-Win64-Shipping.exe", "target process name")
	pid := flag.Int("pid", 0, "target pid (overrides -proc)")
	watch := flag.Bool("watch", true, "tail %TEMP%\\HalcyonA2.log after injecting")
	flag.Parse()

	logPath := filepath.Join(os.TempDir(), "HalcyonA2.log")
	// Clear the log before injecting: the DLL opens it append-mode, so it otherwise
	// accumulates every past run and old crashes drown the current one.
	if err := os.Remove(logPath); err == nil {
		fmt.Println("[a2inject] cleared old", logPath)
	}
	var startOff int64 // fresh file → tail from 0

	target := uint32(*pid)
	if target == 0 {
		p, err := findPID(*proc)
		if err != nil {
			fmt.Fprintln(os.Stderr, "[a2inject] error:", err)
			os.Exit(1)
		}
		target = p
	}
	// Inject a COPY so the build output never stays locked by a running game
	// (a loaded DLL can't be overwritten; copying lets rebuilds proceed freely).
	dllToInject := *dll
	if data, err := os.ReadFile(*dll); err == nil {
		cp := filepath.Join(os.TempDir(), fmt.Sprintf("HalcyonA2_%d.dll", os.Getpid()))
		if os.WriteFile(cp, data, 0o644) == nil {
			dllToInject = cp
		}
	}
	fmt.Printf("[a2inject] target pid=%d dll=%s\n", target, dllToInject)
	if err := inject(target, dllToInject); err != nil {
		fmt.Fprintln(os.Stderr, "[a2inject] inject failed:", err)
		os.Exit(1)
	}
	if *watch {
		tailLog(logPath, startOff)
	}
}
