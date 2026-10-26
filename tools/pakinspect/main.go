// Command pakinspect prints a read-only summary of one or two .pak files
// (versions 8 and 11) and, given two, which entries differ: by stored bytes
// (SHA-1, sizes, compression) and, for small text entries, a unified diff of
// the (decompressed) content.
//
//	go run ./tools/pakinspect [--oodle <dll>] [--context N] <a.pak> [b.pak]
//
// Oodle-compressed entries are decompressed with the DLL given by --oodle,
// PAK_EDITOR_OODLE_DLL, or oo2core_*_win64.dll / oodle-data-shared.dll next
// to the executable.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"solara/internal/pak"
)

const maxTextDiff = 512 << 10

func main() {
	oodlePath := flag.String("oodle", "", "path to the Oodle DLL (oo2core_*_win64.dll or oodle-data-shared.dll)")
	context := flag.Int("context", 2, "unified diff context lines")
	maxList := flag.Int("max", 200, "maximum number of differing entries to list")
	verify := flag.Bool("verify", false, "fully verify each pak (inline headers, SHA-1s, index hashes)")
	flag.Parse()
	if flag.NArg() < 1 || flag.NArg() > 2 {
		fmt.Fprintln(os.Stderr, "usage: pakinspect [--oodle dll] [--context N] <a.pak> [b.pak]")
		os.Exit(2)
	}
	var dec pak.Decompressor
	if path, src := pak.FindOodleDLL(*oodlePath); path != "" {
		o, err := pak.LoadOodle(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Oodle (%s): %v\n", src, err)
		} else {
			fmt.Printf("Oodle: %s (%s)\n", o.Path(), src)
			dec = o
		}
	} else {
		fmt.Println("Oodle: not configured (compressed entries are compared by stored bytes only)")
	}
	a := summary(flag.Arg(0), dec)
	if *verify && a != nil {
		verifyPak(a.Path)
	}
	if flag.NArg() < 2 {
		return
	}
	b := summary(flag.Arg(1), dec)
	if *verify && b != nil {
		verifyPak(b.Path)
	}
	if a == nil || b == nil {
		os.Exit(1)
	}
	compare(a, b, *context, *maxList)
}

func summary(path string, dec pak.Decompressor) *pak.Pak {
	st, err := os.Stat(path)
	if err != nil {
		fmt.Println(path, err)
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		fmt.Println(path, err)
		return nil
	}
	ft, ferr := pak.ReadFooter(f, st.Size())
	f.Close()
	if ferr != nil {
		fmt.Printf("%s: %v\n", path, ferr)
		return nil
	}
	fmt.Printf("%s (%d B): footer v%d (%d B), encryptedIndex=%v, compression=%q\n", path, st.Size(), ft.Version, ft.Size, ft.EncryptedIndex, strings.Join(nonEmpty(ft.CompressionNames), ","))
	p, err := pak.Open(path)
	if err != nil {
		fmt.Printf("  open: %v\n", err)
		return nil
	}
	p.Decompressor = dec
	var comp, enc int
	methods := map[string]int{}
	for _, e := range p.Entries {
		if e.Compressed() {
			comp++
			methods[p.MethodName(e)]++
		}
		if e.Encrypted() {
			enc++
		}
	}
	fmt.Printf("  mount=%q entries=%d compressed=%d %v encrypted=%d index=[%d,+%d)", p.MountPoint, len(p.Entries), comp, methods, enc, ft.IndexOffset, ft.IndexSize)
	if p.Footer.Version >= pak.VersionPathHashIndex {
		fmt.Printf(" pathHashSeed=%#x", p.PathHashSeed())
	}
	fmt.Println()
	for _, e := range p.Entries {
		ln := strings.ToLower(e.Name)
		if strings.HasSuffix(ln, "/config/defaultengine.ini") || strings.HasSuffix(ln, "/config/defaultgame.ini") {
			fmt.Printf("  %s stored=%d raw=%d method=%q blocks=%d sha1=%x editable=%v\n", e.Name, e.Size, e.UncompressedSize, p.MethodName(e), len(e.Blocks), e.Hash[:6], e.Editable())
		}
	}
	return p
}

func nonEmpty(s []string) []string {
	var o []string
	for _, x := range s {
		if x != "" {
			o = append(o, x)
		}
	}
	return o
}

func describe(p *pak.Pak, e *pak.Entry) string {
	m := p.MethodName(e)
	if m == "" {
		m = "none"
	}
	return fmt.Sprintf("stored %d B, raw %d B, %s", e.Size, e.UncompressedSize, m)
}

func compare(a, b *pak.Pak, context, maxList int) {
	bi := map[string]*pak.Entry{}
	for _, e := range b.Entries {
		bi[e.Name] = e
	}
	var onlyA, onlyB []string
	type pair struct{ a, b *pak.Entry }
	var diffs []pair
	samePos := true
	for i, e := range a.Entries {
		o, ok := bi[e.Name]
		if !ok {
			onlyA = append(onlyA, e.Name)
			continue
		}
		if i >= len(b.Entries) || b.Entries[i].Name != e.Name {
			samePos = false
		}
		if e.Hash != o.Hash || e.Size != o.Size || e.UncompressedSize != o.UncompressedSize || a.MethodName(e) != b.MethodName(o) {
			diffs = append(diffs, pair{e, o})
		}
		delete(bi, e.Name)
	}
	for n := range bi {
		onlyB = append(onlyB, n)
	}
	sort.Strings(onlyB)
	fmt.Printf("\n== %d entries differ, %d only in first, %d only in second; entry order identical: %v\n", len(diffs), len(onlyA), len(onlyB), samePos)
	for _, n := range onlyA {
		fmt.Println("  only in first: " + n)
	}
	for _, n := range onlyB {
		fmt.Println("  only in second: " + n)
	}
	for i, d := range diffs {
		if i == maxList {
			fmt.Printf("  ... and %d more\n", len(diffs)-maxList)
			break
		}
		fmt.Printf("  differs: %s\n      first:  %s\n      second: %s\n", d.a.Name, describe(a, d.a), describe(b, d.b))
		if d.a.UncompressedSize > maxTextDiff || d.b.UncompressedSize > maxTextDiff || d.a.Encrypted() || d.b.Encrypted() {
			continue
		}
		da, errA := a.ReadEntry(d.a.Name)
		db, errB := b.ReadEntry(d.b.Name)
		if errA != nil || errB != nil {
			fmt.Printf("      (content not compared: %v %v)\n", errA, errB)
			continue
		}
		if bytes.Equal(da, db) {
			fmt.Println("      content identical (only the stored form differs)")
			continue
		}
		if !isText(da) || !isText(db) {
			fmt.Printf("      binary content differs (%d vs %d bytes)\n", len(da), len(db))
			continue
		}
		for _, l := range unifiedDiff(string(da), string(db), context) {
			fmt.Println("      " + l)
		}
	}
}

func isText(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, c := range b {
		if c < 0x09 || (c > 0x0d && c < 0x20) {
			return false
		}
	}
	return true
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	ls := strings.Split(s, "\n")
	if len(ls) > 0 && ls[len(ls)-1] == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}

// unifiedDiff returns a line diff in unified format (LCS based).
func unifiedDiff(a, b string, context int) []string {
	x, y := splitLines(a), splitLines(b)
	// trim common prefix/suffix to keep the LCS table small
	pre := 0
	for pre < len(x) && pre < len(y) && x[pre] == y[pre] {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && x[len(x)-1-suf] == y[len(y)-1-suf] {
		suf++
	}
	mx, my := x[pre:len(x)-suf], y[pre:len(y)-suf]
	if len(mx)*len(my) > 25_000_000 {
		return []string{fmt.Sprintf("(too many changed lines to diff: %d vs %d)", len(mx), len(my))}
	}
	lcs := make([][]int32, len(mx)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(my)+1)
	}
	for i := len(mx) - 1; i >= 0; i-- {
		for j := len(my) - 1; j >= 0; j-- {
			if mx[i] == my[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	type op struct {
		kind   byte // ' ', '-', '+'
		text   string
		ai, bi int // 0-based line numbers in a and b
	}
	var ops []op
	for i := 0; i < pre; i++ {
		ops = append(ops, op{' ', x[i], i, i})
	}
	i, j := 0, 0
	for i < len(mx) || j < len(my) {
		switch {
		case i < len(mx) && j < len(my) && mx[i] == my[j]:
			ops = append(ops, op{' ', mx[i], pre + i, pre + j})
			i++
			j++
		case i < len(mx) && (j == len(my) || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, op{'-', mx[i], pre + i, pre + j})
			i++
		default:
			ops = append(ops, op{'+', my[j], pre + i, pre + j})
			j++
		}
	}
	for k := 0; k < suf; k++ {
		ops = append(ops, op{' ', x[len(x)-suf+k], len(x) - suf + k, len(y) - suf + k})
	}
	var out []string
	for k := 0; k < len(ops); {
		if ops[k].kind == ' ' {
			k++
			continue
		}
		start := k - context
		if start < 0 {
			start = 0
		}
		end := k
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].kind == ' ' {
				run++
			}
			if run == len(ops) || run-end > 2*context {
				break
			}
			end = run
		}
		stop := end + context
		if stop > len(ops) {
			stop = len(ops)
		}
		na, nb := 0, 0
		for _, o := range ops[start:stop] {
			if o.kind != '+' {
				na++
			}
			if o.kind != '-' {
				nb++
			}
		}
		out = append(out, fmt.Sprintf("@@ -%d,%d +%d,%d @@", ops[start].ai+1, na, ops[start].bi+1, nb))
		for _, o := range ops[start:stop] {
			out = append(out, string(o.kind)+o.text)
		}
		k = stop
	}
	return out
}

func verifyPak(path string) {
	rep, err := pak.Verify(path, nil)
	if err != nil {
		fmt.Printf("  verify: FAILED: %v\n", err)
		return
	}
	fmt.Printf("  verify: OK (%d entries, %d SHA-1s checked, inline headers and index hashes ok)\n", rep.Entries, rep.HashedEntries)
}
