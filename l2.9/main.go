package main

import (
	"errors"
	"fmt"
	"unicode"
)

func unpack(s string) (string, error) {	
	if s == "" {
		return "", nil
	}

	var res string
	var prev rune
	hasLetter := false

	for _, r := range(s) {
		if unicode.IsDigit(r) {
			if prev == -1 {
				return "", errors.New("Incorrect input")
			}
			count := int(r - '0') + 1
			for i := 1; i < count; i++ {
				res += string(prev)
			}
			prev = -1
		} else {
			hasLetter = true
			if prev != -1 {
				res += string(prev)
			}
			prev = r
		}
	}

	if prev != -1 {
		res += string(prev)
	}
	if !hasLetter {
		return "", errors.New("Incorrect input")
	}

	return res, nil
}


func main() {
	fmt.Println(unpack("a4bc2d5e"))
	fmt.Println(unpack("abcd"))
	fmt.Println(unpack("67"))
	fmt.Println(unpack(""))

}