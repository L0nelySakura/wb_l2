package main

import (
	"strings"
	"flag"
	"fmt"
	"os"
	"strconv"
	"bufio"
)

type FieldRange struct {
    start int
    end   int
}

type Config struct {
	fields []FieldRange
	delimiter string
	separated bool

}


func parseFlags() *Config {
	cfg := &Config{}

	fieldsStr := flag.String("f", "", "номера полей (например: 1,3-5,7)")
	delimiter := flag.String("d", "\t", "разделитель полей (по умолчанию табуляция)")
	separated := flag.Bool("s", false, "только строки с разделителем")

	flag.Parse()

	if *fieldsStr == "" {
		fmt.Fprintln(os.Stderr, "ERROR: flag -f is required!")
		flag.Usage()
		os.Exit(1)
	}

	var err error
	cfg.fields, err = parseFields(*fieldsStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: invalid fields format: %v\n", err)
		os.Exit(1)
	}
	cfg.delimiter = *delimiter
	cfg.separated = *separated

	return cfg
}


func parseFields(fieldsStr string) ([]FieldRange, error) {
	if fieldsStr == "" {
		return nil, fmt.Errorf("fields are required")
	}

	var ranges []FieldRange
	parts := strings.Split(fieldsStr, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "-") {
			r, err := parseRange(part)
			if err != nil {
				return nil, err
			}
			ranges = append(ranges, r)
		} else {
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid field number: %s", part)
			}
			if num <= 0 {
				return nil, fmt.Errorf("field numbers must be positive: %d", num)
			}
			ranges = append(ranges, FieldRange{start: num, end: num})
		}
	}

	return ranges, nil
}

func parseRange(rangeStr string) (FieldRange, error) {
	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return FieldRange{}, fmt.Errorf("invalid range format: %s", rangeStr)
	}

	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return FieldRange{}, fmt.Errorf("invalid range start: %s", parts[0])
	}

	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return FieldRange{}, fmt.Errorf("invalid range end: %s", parts[1])
	}

	if start <= 0 || end <= 0 {
		return FieldRange{}, fmt.Errorf("range values must be positive: %d-%d", start, end)
	}

	if start > end {
		return FieldRange{}, fmt.Errorf("range start (%d) > end (%d)", start, end)
	}

	return FieldRange{start: start, end: end}, nil
}


func processStdin(cfg *Config) error {
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()

		if cfg.separated && !strings.Contains(line, cfg.delimiter) {
			continue
		}

		current_fields := strings.Split(line, cfg.delimiter)
		current_lenght := len(current_fields)
		var output []string

		for _, r := range cfg.fields {

			for num := r.start; num <= r.end; num++ {
				if num - 1 < current_lenght {
					output = append(output, current_fields[num - 1])
				} else {
					break
				}
			} 
		} 
		if len(output) > 0 {
			fmt.Println(output)
		}
	}
	return scanner.Err()
}


func main() {
	cfg := parseFlags()

	err := processStdin(cfg)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}
}