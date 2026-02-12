package main

import (
	"fmt"
	"sort"
	"strings"
)

func anagram(words []string) map[string][]string {
	tmpMap := make(map[string][]string)
	for _, word := range words {
		lowered := strings.ToLower(word)

		runes := []rune(lowered)
		sort.Slice(runes, func(i, j int) bool {
			return runes[i] < runes[j]
		})
		key := string(runes)
		tmpMap[key] = append(tmpMap[key], word)
	}
	resultMap := make(map[string][]string)
	for _, anagrams := range tmpMap {
		if len(anagrams) >= 2 {
			resultMap[anagrams[0]] = anagrams
		}
	}
	return resultMap
}

func main() {
	words :=  []string{"пятак", "пятка", "тяпка", "листок", "слиток", "столик", "стол"}

	fmt.Println(anagram(
		words,
	))
}