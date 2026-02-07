package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	keyColumn int
	numeric   bool 
	reverse   bool
	unique    bool
}

type Record struct {
	original string 
	key      string
	numKey   float64
	hasNum   bool
}

type Sorter struct {
	config Config
	lines  []Record
}
func NewSorter(config Config) *Sorter {
	return &Sorter{
		config: config,
		lines:  make([]Record, 0),
	}
}

func (s *Sorter) ReadLines(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("ошибка открытия файла: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		record := Record{
			original: line,
		}
		if s.config.keyColumn > 0 {
			record.key = s.extractColumn(line, s.config.keyColumn)
		} else {
			record.key = line
		}
		if s.config.numeric {
			s.parseNumericKey(&record)
		}

		s.lines = append(s.lines, record)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ошибка чтения файла: %v", err)
	}

	return nil
}

func (s *Sorter) extractColumn(line string, col int) string {
	columns := strings.Fields(line)

	if col > 0 && col <= len(columns) {
		return columns[col-1]
	}

	return ""
}

func (s *Sorter) parseNumericKey(record *Record) {
	key := record.key
	key = strings.TrimSpace(key)
	
	if num, err := strconv.ParseFloat(key, 64); err == nil {
		record.numKey = num
		record.hasNum = true
	} else {
		record.hasNum = false
		record.numKey = 0
	}
}

func (s *Sorter) Less(i, j int) bool {
	a, b := s.lines[i], s.lines[j]
	if s.config.numeric {
		if a.hasNum && b.hasNum {
			if s.config.reverse {
				return a.numKey > b.numKey
			}
			return a.numKey < b.numKey
		}

		if a.hasNum && !b.hasNum {
			if s.config.reverse {
				return false
			}
			return true
		}
		if !a.hasNum && b.hasNum {
			if s.config.reverse {
				return true
			}
			return false
		}
	}
	if s.config.reverse {
		return a.key > b.key
	}
	return a.key < b.key
}

func (s *Sorter) Swap(i, j int) {
	s.lines[i], s.lines[j] = s.lines[j], s.lines[i]
}

func (s *Sorter) Len() int {
	return len(s.lines)
}

func (s *Sorter) Sort() {
	sort.Sort(s)
}

func (s *Sorter) WriteLines(filename string) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return fmt.Errorf("ошибка создания директории: %v", err)
	}
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("ошибка создания файла: %v", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	seen := make(map[string]bool)
	for _, record := range s.lines {
		line := record.original
		if s.config.unique {
			if seen[line] {
				continue
			}
			seen[line] = true
		}

		if _, err := writer.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("ошибка записи в файл: %v", err)
		}
	}
	return nil
}

func parseFlags() (Config, string) {
	var config Config

	flag.IntVar(&config.keyColumn, "k", 0, "column number to sort by (1-indexed)")
	flag.BoolVar(&config.numeric, "n", false, "sort numerically")
	flag.BoolVar(&config.reverse, "r", false, "sort in reverse order")
	flag.BoolVar(&config.unique, "u", false, "output only unique lines")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] <filename>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Sort lines of text from input file and save to output directory\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nInput file must be in 'input' directory\n")
		fmt.Fprintf(os.Stderr, "Output will be saved to 'output' directory with same filename\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s data.txt                 # sort entire lines as strings\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -k 2 data.txt            # sort by 2nd column as strings\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -k 2 -n data.txt         # sort by 2nd column numerically\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -n data.txt              # sort entire lines numerically\n", os.Args[0])
	}

	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "Error: input file is required\n")
		flag.Usage()
		os.Exit(1)
	}

	filename := flag.Arg(0)
	return config, filename
}

func createOutputFilename(inputFilename string) (string, error) {
	baseName := filepath.Base(inputFilename)
	outputPath := filepath.Join("output", baseName)
	
	return outputPath, nil
}

func ensureDirectories() error {
	if _, err := os.Stat("input"); os.IsNotExist(err) {
		return fmt.Errorf("директория 'input' не существует")
	}
	if err := os.MkdirAll("output", 0755); err != nil {
		return fmt.Errorf("не удалось создать директорию 'output': %v", err)
	}

	return nil
}

func processFile(config Config, inputFilename string) error {
	if _, err := os.Stat(inputFilename); os.IsNotExist(err) {
		return fmt.Errorf("файл не найден: %s", inputFilename)
	}
	sorter := NewSorter(config)
	if err := sorter.ReadLines(inputFilename); err != nil {
		return fmt.Errorf("ошибка чтения файла: %v", err)
	}

	sorter.Sort()

	outputFilename, err := createOutputFilename(inputFilename)
	if err != nil {
		return fmt.Errorf("ошибка создания имени выходного файла: %v", err)
	}
	if err := sorter.WriteLines(outputFilename); err != nil {
		return fmt.Errorf("ошибка записи в файл: %v", err)
	}

	fmt.Printf("Файл успешно обработан: %s -> %s\n", inputFilename, outputFilename)
	return nil
}

func main() {
	config, inputArg := parseFlags()
	if err := ensureDirectories(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	inputFilename := filepath.Join("input", inputArg)

	if err := processFile(config, inputFilename); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}