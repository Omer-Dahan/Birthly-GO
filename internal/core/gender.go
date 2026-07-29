package core

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed names.json
var namesJSON []byte

type nameLists struct {
	m      map[string]struct{}
	f      map[string]struct{}
	unisex map[string]struct{}
	mList  []string
	fList  []string
}

var (
	namesOnce sync.Once
	names     nameLists
)

func loadNames() {
	namesOnce.Do(func() {
		var raw struct {
			M      []string `json:"m"`
			F      []string `json:"f"`
			Unisex []string `json:"unisex"`
		}
		if err := json.Unmarshal(namesJSON, &raw); err != nil {
			names = nameLists{m: map[string]struct{}{}, f: map[string]struct{}{}, unisex: map[string]struct{}{}}
			return
		}
		names.m = make(map[string]struct{}, len(raw.M))
		for _, n := range raw.M {
			names.m[n] = struct{}{}
		}
		names.f = make(map[string]struct{}, len(raw.F))
		for _, n := range raw.F {
			names.f[n] = struct{}{}
		}
		names.unisex = make(map[string]struct{}, len(raw.Unisex))
		for _, n := range raw.Unisex {
			names.unisex[n] = struct{}{}
		}
		names.mList = raw.M
		names.fList = raw.F
	})
}

// DetectGender auto-detects whether name is a man's or woman's name. Cleans
// the string (takes the first word) and checks against the CBS-derived name
// database. Returns "m", "f", or ("" , false) if unknown or unisex.
//
// Matches app/core/gender_detector.py exactly, including its use of
// difflib.get_close_matches (Ratcliff/Obershelp ratio, NOT Levenshtein or
// Jaro-Winkler — a different algorithm gives different results on
// borderline misspellings) at cutoff 0.85 for names longer than 2 runes.
func DetectGender(name string) (string, bool) {
	loadNames()

	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "", false
	}
	firstName := parts[0]

	if _, ok := names.unisex[firstName]; ok {
		return "", false
	}
	if _, ok := names.m[firstName]; ok {
		return GenderMale, true
	}
	if _, ok := names.f[firstName]; ok {
		return GenderFemale, true
	}

	if len([]rune(firstName)) > 2 {
		_, mOK := closestMatch(firstName, names.mList, 0.85)
		_, fOK := closestMatch(firstName, names.fList, 0.85)
		if mOK && !fOK {
			return GenderMale, true
		}
		if fOK && !mOK {
			return GenderFemale, true
		}
	}

	return "", false
}

// closestMatch ports Python's difflib.get_close_matches(word, possibilities,
// n=1, cutoff). Candidates are gated by sequenceRatio(candidate, word) (note
// the argument order: Python's SequenceMatcher has set_seq1(candidate) /
// set_seq2(word), i.e. a=candidate, b=word — ratio() is not perfectly
// symmetric in its tie-breaking so this order must match). Among candidates
// at or above cutoff, Python's heapq.nlargest(1, [(ratio, x), ...]) picks
// the highest ratio, breaking ties by the lexicographically GREATEST
// candidate string (tuple comparison falls through to the second element).
func closestMatch(word string, possibilities []string, cutoff float64) (string, bool) {
	wordRunes := []rune(word)
	bestRatio := -1.0
	best := ""
	found := false
	for _, candidate := range possibilities {
		r := sequenceRatio([]rune(candidate), wordRunes)
		if r < cutoff {
			continue
		}
		if r > bestRatio || (r == bestRatio && candidate > best) {
			bestRatio = r
			best = candidate
			found = true
		}
	}
	return best, found
}

// sequenceRatio computes the Ratcliff/Obershelp similarity ratio between a
// and b: 2*M / T, where M is the total length of all matching blocks found
// by the classic recursive longest-matching-block algorithm (Python's
// difflib.SequenceMatcher with no junk/autojunk — never triggered here since
// autojunk only applies to sequences of 200+ elements, far beyond any name),
// and T is len(a)+len(b).
func sequenceRatio(a, b []rune) float64 {
	total := len(a) + len(b)
	if total == 0 {
		return 1.0
	}
	b2j := buildB2J(b)
	matches := sumMatchingBlocks(a, b, b2j, 0, len(a), 0, len(b))
	return 2.0 * float64(matches) / float64(total)
}

func buildB2J(b []rune) map[rune][]int {
	b2j := make(map[rune][]int)
	for j, r := range b {
		b2j[r] = append(b2j[r], j)
	}
	return b2j
}

// sumMatchingBlocks recursively sums the sizes of all matching blocks in
// a[alo:ahi] vs b[blo:bhi], mirroring SequenceMatcher.get_matching_blocks.
func sumMatchingBlocks(a, b []rune, b2j map[rune][]int, alo, ahi, blo, bhi int) int {
	besti, bestj, bestsize := findLongestMatch(a, b, b2j, alo, ahi, blo, bhi)
	if bestsize == 0 {
		return 0
	}
	total := bestsize
	if alo < besti && blo < bestj {
		total += sumMatchingBlocks(a, b, b2j, alo, besti, blo, bestj)
	}
	if besti+bestsize < ahi && bestj+bestsize < bhi {
		total += sumMatchingBlocks(a, b, b2j, besti+bestsize, ahi, bestj+bestsize, bhi)
	}
	return total
}

// findLongestMatch is a direct port of
// difflib.SequenceMatcher.find_longest_match (junk/autojunk-free path).
func findLongestMatch(a, b []rune, b2j map[rune][]int, alo, ahi, blo, bhi int) (besti, bestj, bestsize int) {
	besti, bestj, bestsize = alo, blo, 0
	j2len := make(map[int]int)
	for i := alo; i < ahi; i++ {
		newj2len := make(map[int]int)
		for _, j := range b2j[a[i]] {
			if j < blo {
				continue
			}
			if j >= bhi {
				break
			}
			k := j2len[j-1] + 1
			newj2len[j] = k
			if k > bestsize {
				besti, bestj, bestsize = i-k+1, j-k+1, k
			}
		}
		j2len = newj2len
	}

	for besti > alo && bestj > blo && a[besti-1] == b[bestj-1] {
		besti--
		bestj--
		bestsize++
	}
	for besti+bestsize < ahi && bestj+bestsize < bhi && a[besti+bestsize] == b[bestj+bestsize] {
		bestsize++
	}

	return besti, bestj, bestsize
}
