// Port of the parts of FuzzySharp 2.0.2 (the NuGet package A2StationDbServer.cs
// uses for the user search_string filter) reachable from
// Process.ExtractTop(query, choices) with its defaults: the Full string
// preprocessor, the WeightedRatioScorer, limit 5, cutoff 0, and the MinHeap
// based MaxN selection. Ported from the decompiled FuzzySharp.dll shipped in
// the original build output; verified against it by fuzzysharp_test.go.
package ares

import (
	"math"
	"sort"
	"strings"
	"unicode/utf16"

	"solara/internal/dotnet"
)

// a2FuzzyResult mirrors ExtractedResult<string>.
type a2FuzzyResult struct {
	Value *string
	Score int
	Index int
}

// a2ExtractTop mirrors Process.ExtractTop(query, choices) (limit 5, cutoff 0),
// returning results best-first as the original enumerates them.
func a2ExtractTop(query string, choices []*string) []a2FuzzyResult {
	const limit = 5
	pq := a2FuzzyPreprocess(&query)
	h := &a2MinHeap{}
	for i, ch := range choices {
		score := a2WeightedRatio(pq, a2FuzzyPreprocess(ch))
		r := a2FuzzyResult{Value: ch, Score: score, Index: i}
		// MaxN: fill to n, then replace the minimum only on a strictly greater score.
		if h.count() < limit {
			h.add(r)
		} else if r.Score > h.getMin().Score {
			h.extractDominating()
			h.add(r)
		}
	}
	var popped []a2FuzzyResult
	for i := 0; i < limit && h.count() > 0; i++ {
		popped = append(popped, h.extractDominating())
	}
	// .Reverse()
	for i, j := 0, len(popped)-1; i < j; i, j = i+1, j-1 {
		popped[i], popped[j] = popped[j], popped[i]
	}
	return popped
}

// a2FuzzyPreprocess mirrors StringPreprocessorFactory.Default:
// Regex.Replace(input, "[^ a-zA-Z0-9]", " ").ToLower().Trim(). Each UTF-16
// code unit is replaced, so a supplementary character becomes two spaces.
// A null input throws ArgumentNullException, as Regex.Replace does.
func a2FuzzyPreprocess(s *string) string {
	if s == nil {
		dotnet.Throw("System.ArgumentNullException", "Value cannot be null. (Parameter 'input')")
	}
	var b strings.Builder
	for _, r := range *s {
		switch {
		case r == ' ' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= 0x10000:
			b.WriteString("  ")
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Trim(b.String(), " ")
}

// a2WeightedRatio mirrors WeightedRatioScorer.Score.
func a2WeightedRatio(input1, input2 string) int {
	len1, len2 := len(input1), len(input2) // ASCII after preprocessing
	if len1 == 0 || len2 == 0 {
		return 0
	}
	tryPartials := true
	const unbaseScale = 0.95
	partialScale := 0.9
	baseRatio := a2FuzzRatio(input1, input2)
	lenRatio := float64(max(len1, len2)) / float64(min(len1, len2))
	if lenRatio < 1.5 {
		tryPartials = false
	}
	if lenRatio > 8.0 {
		partialScale = 0.6
	}
	if tryPartials {
		partial := float64(a2FuzzPartialRatio(input1, input2)) * partialScale
		partialSor := float64(a2FuzzTokenSortRatio(input1, input2)) * unbaseScale * partialScale
		partialSet := float64(a2FuzzTokenSetRatio(input1, input2)) * unbaseScale * partialScale
		return int(math.RoundToEven(math.Max(math.Max(float64(baseRatio), partial), math.Max(partialSor, partialSet))))
	}
	tokenSort := float64(a2FuzzTokenSortRatio(input1, input2)) * unbaseScale
	tokenSet := float64(a2FuzzTokenSetRatio(input1, input2)) * unbaseScale
	return int(math.RoundToEven(math.Max(float64(baseRatio), math.Max(tokenSort, tokenSet))))
}

// a2FuzzRatio mirrors DefaultRatioStrategy.Calculate.
func a2FuzzRatio(a, b string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	return int(math.RoundToEven(100.0 * a2LevRatio(a2Units(a), a2Units(b))))
}

// a2FuzzPartialRatio mirrors PartialRatioStrategy.Calculate.
func a2FuzzPartialRatio(input1, input2 string) int {
	if len(input1) == 0 || len(input2) == 0 {
		return 0
	}
	shorter, longer := a2Units(input2), a2Units(input1)
	if len(a2Units(input1)) < len(a2Units(input2)) {
		shorter, longer = a2Units(input1), a2Units(input2)
	}
	blocks := a2MatchingBlocks(shorter, longer)
	best := math.Inf(-1)
	for _, mb := range blocks {
		dist := mb.DestPos - mb.SourcePos
		start := 0
		if dist > 0 {
			start = dist
		}
		end := start + len(shorter)
		if end > len(longer) {
			end = len(longer)
		}
		r := a2LevRatio(shorter, longer[start:end])
		if r > 0.995 {
			return 100
		}
		best = math.Max(best, r)
	}
	return int(math.RoundToEven(100.0 * best))
}

// a2Tokens mirrors Regex.Split(s, "\\s+").Where(s => s.Any()).
func a2Tokens(s string) []string { return strings.Fields(s) }

// a2FuzzTokenSortRatio mirrors TokenSortScorer.
func a2FuzzTokenSortRatio(input1, input2 string) int {
	sorted := func(s string) string {
		t := a2Tokens(s)
		sort.Strings(t) // invariant-mode default comparer on [a-z0-9] tokens == ordinal
		return strings.Trim(strings.Join(t, " "), " ")
	}
	return a2FuzzRatio(sorted(input1), sorted(input2))
}

// a2FuzzTokenSetRatio mirrors TokenSetScorer.
func a2FuzzTokenSetRatio(input1, input2 string) int {
	set := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, t := range a2Tokens(s) {
			m[t] = true
		}
		return m
	}
	s1, s2 := set(input1), set(input2)
	var inter, diff12, diff21 []string
	for t := range s1 {
		if s2[t] {
			inter = append(inter, t)
		} else {
			diff12 = append(diff12, t)
		}
	}
	for t := range s2 {
		if !s1[t] {
			diff21 = append(diff21, t)
		}
	}
	sort.Strings(inter)
	sort.Strings(diff12)
	sort.Strings(diff21)
	text := strings.Trim(strings.Join(inter, " "), " ")
	text2 := strings.Trim(text+" "+strings.Join(diff12, " "), " ")
	arg := strings.Trim(text+" "+strings.Join(diff21, " "), " ")
	return max(a2FuzzRatio(text, text2), a2FuzzRatio(text, arg), a2FuzzRatio(text2, arg))
}

func a2Units(s string) []uint16 { return utf16.Encode([]rune(s)) }

// a2LevRatio mirrors Levenshtein.GetRatio: (lensum - indel distance) / lensum,
// where the distance uses EditDistance(xcost: 1) (substitution costs 2).
func a2LevRatio(a, b []uint16) float64 {
	lensum := len(a) + len(b)
	d := a2IndelDistance(a, b)
	if d == 0 {
		return 1.0
	}
	return float64(lensum-d) / float64(lensum)
}

// a2IndelDistance is the edit distance with insert/delete cost 1 and
// substitution cost 2 (= len(a)+len(b)-2*LCS), what EditDistance(xcost=1) computes.
func a2IndelDistance(a, b []uint16) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			sub := prev[j-1] + 2
			if a[i-1] == b[j-1] {
				sub = prev[j-1]
			}
			cur[j] = min(sub, prev[j]+1, cur[j-1]+1)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

type a2EditType int

const (
	a2Delete a2EditType = iota
	a2Equal
	a2Insert
	a2Replace
	a2Keep
)

type a2EditOp struct {
	EditType  a2EditType
	SourcePos int
	DestPos   int
}

type a2MatchingBlock struct {
	SourcePos, DestPos, Length int
}

// a2EditOps mirrors Levenshtein.GetEditOps(len1, c1, len2, c2).
func a2EditOps(c1, c2 []uint16) []a2EditOp {
	len1, len2 := len(c1), len(c2)
	p1, p2, o := 0, 0, 0
	for len1 > 0 && len2 > 0 && c1[p1] == c2[p2] {
		len1--
		len2--
		p1++
		p2++
		o++
	}
	o2 := o
	for len1 > 0 && len2 > 0 && c1[p1+len1-1] == c2[p2+len2-1] {
		len1--
		len2--
	}
	len1++
	len2++
	m := make([]int, len2*len1)
	for i := 0; i < len2; i++ {
		m[i] = i
	}
	for i := 1; i < len1; i++ {
		m[len2*i] = i
	}
	for i := 1; i < len1; i++ {
		prev := (i - 1) * len2
		p := i * len2
		end := p + len2 - 1
		ch := c1[p1+i-1]
		c2p := p2
		x := i
		p++
		for p <= end {
			c := m[prev]
			prev++
			if ch != c2[c2p] {
				c++
			}
			c2p++
			x++
			if x > c {
				x = c
			}
			c = m[prev] + 1
			if x > c {
				x = c
			}
			m[p] = x
			p++
		}
	}
	return a2EditOpsFromCostMatrix(len1, c1, p1, o, len2, c2, p2, o2, m)
}

// a2EditOpsFromCostMatrix mirrors Levenshtein.EditOpsFromCostMatrix.
func a2EditOpsFromCostMatrix(len1 int, c1 []uint16, p1, o1, len2 int, c2 []uint16, p2, o2 int, matrix []int) []a2EditOp {
	dir := 0
	pos := matrix[len1*len2-1]
	ops := make([]a2EditOp, pos)
	i := len1 - 1
	j := len2 - 1
	p := len1*len2 - 1
	for i > 0 || j > 0 {
		if i != 0 && j != 0 && matrix[p] == matrix[p-len2-1] && c1[p1+i-1] == c2[p2+j-1] {
			i--
			j--
			p -= len2 + 1
			dir = 0
			continue
		}
		if dir < 0 && j != 0 && matrix[p] == matrix[p-1]+1 {
			pos--
			j--
			ops[pos] = a2EditOp{a2Insert, i + o1, j + o2}
			p--
			continue
		}
		if dir > 0 && i != 0 && matrix[p] == matrix[p-len2]+1 {
			pos--
			i--
			ops[pos] = a2EditOp{a2Delete, i + o1, j + o2}
			p -= len2
			continue
		}
		if i != 0 && j != 0 && matrix[p] == matrix[p-len2-1]+1 {
			pos--
			i--
			j--
			ops[pos] = a2EditOp{a2Replace, i + o1, j + o2}
			p -= len2 + 1
			dir = 0
			continue
		}
		if dir == 0 && j != 0 && matrix[p] == matrix[p-1]+1 {
			pos--
			j--
			ops[pos] = a2EditOp{a2Insert, i + o1, j + o2}
			p--
			dir = -1
			continue
		}
		if dir == 0 && i != 0 && matrix[p] == matrix[p-len2]+1 {
			pos--
			i--
			ops[pos] = a2EditOp{a2Delete, i + o1, j + o2}
			p -= len2
			dir = 1
			continue
		}
		dotnet.Throw("System.InvalidOperationException", "Cant calculate edit op")
	}
	return ops
}

// a2MatchingBlocks mirrors Levenshtein.GetMatchingBlocks(s1, s2).
func a2MatchingBlocks(s1, s2 []uint16) []a2MatchingBlock {
	len1, len2 := len(s1), len(s2)
	ops := a2EditOps(s1, s2)
	n := len(ops)
	// The original makes two identical passes (count, then fill); one pass
	// collecting the blocks is equivalent.
	var blocks []a2MatchingBlock
	i, sp, dp := 0, 0, 0
	left := n
	for left != 0 {
		for ops[i].EditType == a2Keep {
			left--
			if left == 0 {
				break
			}
			i++
		}
		if left == 0 {
			break
		}
		if sp < ops[i].SourcePos || dp < ops[i].DestPos {
			blocks = append(blocks, a2MatchingBlock{sp, dp, ops[i].SourcePos - sp})
			sp = ops[i].SourcePos
			dp = ops[i].DestPos
		}
		et := ops[i].EditType
		switch et {
		case a2Replace:
			for {
				sp++
				dp++
				left--
				i++
				if !(left != 0 && ops[i].EditType == et && sp == ops[i].SourcePos && dp == ops[i].DestPos) {
					break
				}
			}
		case a2Delete:
			for {
				sp++
				left--
				i++
				if !(left != 0 && ops[i].EditType == et && sp == ops[i].SourcePos && dp == ops[i].DestPos) {
					break
				}
			}
		case a2Insert:
			for {
				dp++
				left--
				i++
				if !(left != 0 && ops[i].EditType == et && sp == ops[i].SourcePos && dp == ops[i].DestPos) {
					break
				}
			}
		}
	}
	if sp < len1 || dp < len2 {
		blocks = append(blocks, a2MatchingBlock{sp, dp, len1 - sp})
	}
	blocks = append(blocks, a2MatchingBlock{len1, len2, 0})
	return blocks
}

// a2MinHeap mirrors FuzzySharp.Utils.MinHeap<ExtractedResult<string>> with
// Dominates(x, y) = x.Score <= y.Score.
type a2MinHeap struct{ h []a2FuzzyResult }

func (m *a2MinHeap) count() int              { return len(m.h) }
func (m *a2MinHeap) dominates(x, y int) bool { return m.h[x].Score <= m.h[y].Score }
func (m *a2MinHeap) swap(i, j int)           { m.h[i], m.h[j] = m.h[j], m.h[i] }
func (m *a2MinHeap) getMin() a2FuzzyResult   { return m.h[0] }
func a2HeapParent(i int) int                 { return (i+1)/2 - 1 }
func a2HeapYoung(i int) int                  { return (i+1)*2 - 1 }
func (m *a2MinHeap) add(r a2FuzzyResult)     { m.h = append(m.h, r); m.bubbleUp(len(m.h) - 1) }

func (m *a2MinHeap) bubbleUp(i int) {
	for i != 0 && !m.dominates(a2HeapParent(i), i) {
		m.swap(i, a2HeapParent(i))
		i = a2HeapParent(i)
	}
}

func (m *a2MinHeap) extractDominating() a2FuzzyResult {
	res := m.h[0]
	tail := len(m.h) - 1
	m.swap(tail, 0)
	m.h = m.h[:tail]
	m.bubbleDown(0)
	return res
}

func (m *a2MinHeap) bubbleDown(i int) {
	for {
		d := m.dominating(i)
		if d == i {
			return
		}
		m.swap(i, d)
		i = d
	}
}

func (m *a2MinHeap) dominating(i int) int {
	d := i
	d = m.getDominating(a2HeapYoung(i), d)
	return m.getDominating(a2HeapYoung(i)+1, d)
}

func (m *a2MinHeap) getDominating(n, d int) int {
	if n < len(m.h) && !m.dominates(d, n) {
		return n
	}
	return d
}
