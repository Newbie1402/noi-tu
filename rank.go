package main

import (
	"sort"
	"strings"
)

type rankedWord struct {
	Word            string `json:"word"`
	Verified        bool   `json:"verified"`
	OpponentReplies int    `json:"opponent_replies"`
	WorstNextMoves  int    `json:"worst_next_moves"`
	Assessment      string `json:"assessment"`
}

// rankWords estimates two plies using the available dictionary, not the target server's full word set.
func rankWords(key string, dictionary map[string][]string, observed map[string]bool, used map[string]bool) []rankedWord {
	index := make(map[string][]string, len(dictionary))
	seen := map[string]bool{}
	add := func(word string) {
		word = normalize(word)
		first, ok := parts(word)
		if !ok || seen[word] || used[word] {
			return
		}
		if accepted, known := observed[word]; known && !accepted {
			return
		}
		seen[word] = true
		index[first] = append(index[first], word)
	}
	for _, words := range dictionary {
		for _, word := range words {
			add(word)
		}
	}
	for word, accepted := range observed {
		if accepted {
			add(word)
		}
	}
	result := make([]rankedWord, 0, len(index[key]))
	for _, word := range index[key] {
		last := strings.Fields(word)[1]
		replies := index[last]
		count, worst := 0, -1
		for _, reply := range replies {
			if reply == word {
				continue
			}
			count++
			next := 0
			for _, continuation := range index[strings.Fields(reply)[1]] {
				if continuation != word && continuation != reply {
					next++
				}
			}
			if worst < 0 || next < worst {
				worst = next
			}
		}
		assessment := "risk"
		if count == 0 {
			assessment = "possible_kill"
		} else if worst > 0 {
			assessment = "has_reply"
		}
		result = append(result, rankedWord{word, observed[word], count, worst, assessment})
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		category := func(w rankedWord) int {
			if w.OpponentReplies == 0 {
				return 0
			}
			if w.WorstNextMoves > 0 {
				return 1
			}
			return 2
		}
		if category(a) != category(b) {
			return category(a) < category(b)
		}
		if a.Verified != b.Verified {
			return a.Verified
		}
		if a.OpponentReplies != b.OpponentReplies {
			return a.OpponentReplies < b.OpponentReplies
		}
		if a.WorstNextMoves != b.WorstNextMoves {
			return a.WorstNextMoves > b.WorstNextMoves
		}
		return a.Word < b.Word
	})
	return result
}
