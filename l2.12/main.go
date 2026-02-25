package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
)

type Config struct {
	before int
	after int
	context int

	count bool
	ignore bool
	invert bool
	fixed bool
	lineNum bool

	pattern string
	files []string
}


func parseFlags() *Config{
	config := &Config{}

	flag.IntVar(&config.after, "A", 0, "N lines after match")
	flag.IntVar(&config.before, "B", 0, "N lines before match")
	flag.IntVar(&config.context, "C", 0, "N lines around match")

	flag.BoolVar(&config.count, "c", false, "print only count of matching lines")
	flag.BoolVar(&config.ignore, "i", false, "ignore case distinctions")
	flag.BoolVar(&config.invert, "v", false, "invert search")
	flag.BoolVar(&config.fixed, "F", false, "fixed match")
	flag.BoolVar(&config.lineNum, "n", false, "print line numbers")

	flag.Parse()

	if config.context > 0 {
		config.after = config.context
		config.before = config.context
	}
	if flag.NArg() == 0 {
		config.pattern = ""
	} else {
		config.pattern = flag.Arg(0)
		config.files = flag.Args()[1:]
	}

	return config
}

func processFile(config *Config, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("cannot open file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	err = scanner.Err()
	if err != nil {
		return fmt.Errorf("error reading file: %v", err)
	}
	if len(lines) != 0 {

		matches, err := findMatches(config, lines)
		if err != nil {
			return fmt.Errorf("error in: %v", err)
		}
		printResults(config, filename, lines, matches)
	}
	return nil
}

func findMatches(config *Config, lines []string) ([]int, error) {
	pattern := config.pattern
	if config.ignore {
		pattern = "(?i)" + pattern
	}

	var re *regexp.Regexp
	var err error
	if config.fixed {
		re, err = regexp.Compile(regexp.QuoteMeta(pattern))
	} else {
		re, err = regexp.Compile(pattern)
	}
	if err != nil {
		return []int{}, fmt.Errorf("invalid pattern: %v", err)
	}

	var matches []int
	for i, line := range lines {
		match := re.MatchString(line)
		if config.invert {
			match = !match
		}
		if match {
			matches = append(matches, i)
		}
	}
	
	return matches, nil
}

func printResults(config *Config, filename string, lines []string, matches []int) {
	if config.count {
		fmt.Printf("%s:%d\n", filename, len(matches))
		return
	}
	fmt.Printf("%s:\n", filename)
	defer fmt.Println("")
	if len(matches) == 0 {
		return
	}

	lastPrintedIdx := -1
	for _, matchIdx := range matches {

		start := matchIdx - config.before
		if start < 0 {
			start = 0
		}
		
		end := matchIdx + config.after
		if end >= len(lines) {
			end = len(lines) - 1
		}

		if lastPrintedIdx != -1 && start > lastPrintedIdx+1 {
			fmt.Println("--")
		}

		printStart := start
		if lastPrintedIdx != -1 && start <= lastPrintedIdx {
			printStart = lastPrintedIdx + 1
		}
		for idx := printStart; idx <= end; idx++ {
			prefix := ""
			if config.lineNum {
				prefix = fmt.Sprintf("%d:", idx+1)
			}
			
			if prefix != "" {
				fmt.Printf("%s%s\n", prefix, lines[idx])
			} else {
				fmt.Println(lines[idx])
			}
			
			lastPrintedIdx = idx
		}
	}

	
}


func main() {
	config := parseFlags()
	
	if config.pattern == "" {
		fmt.Println("ERROR: pattern is required!")
		os.Exit(1) 
	}
	if len(config.files) == 0 {
		fmt.Println("ERROR: no files specified!")
		os.Exit(1) 
	}

	for _, filename := range config.files {

		err := processFile(config, filename) 
		if err != nil {
			fmt.Printf("ERROR: error in processing file %s: %v\n", filename, err)
			os.Exit(1)
		}
	}
}
// example
// go run .\main.go -A 1 -B 1 -n -c "4" 1.txt 2.txt 
