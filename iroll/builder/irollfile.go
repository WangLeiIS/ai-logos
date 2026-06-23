package builder

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type InstructionType int

const (
	InstFrom InstructionType = iota
	InstMigrate
	InstCopy
)

type Instruction struct {
	Type InstructionType
	Args []string
}

type Irollfile struct {
	Instructions []Instruction
	Dir          string
}

func ParseIrollfile(path string) (*Irollfile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open irollfile: %w", err)
	}
	defer f.Close()

	absPath, _ := filepath.Abs(path)
	dir := filepath.Dir(absPath)

	lf := &Irollfile{Dir: dir}
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		switch strings.ToUpper(parts[0]) {
		case "FROM":
			if len(parts) != 2 {
				return nil, fmt.Errorf("line %d: FROM requires exactly 1 argument", lineNum)
			}
			lf.Instructions = append(lf.Instructions, Instruction{Type: InstFrom, Args: []string{parts[1]}})

		case "MIGRATE":
			if len(parts) != 2 {
				return nil, fmt.Errorf("line %d: MIGRATE requires exactly 1 argument", lineNum)
			}
			lf.Instructions = append(lf.Instructions, Instruction{Type: InstMigrate, Args: []string{parts[1]}})

		case "COPY":
			if len(parts) != 3 {
				return nil, fmt.Errorf("line %d: COPY requires exactly 2 arguments", lineNum)
			}
			lf.Instructions = append(lf.Instructions, Instruction{Type: InstCopy, Args: []string{parts[1], parts[2]}})

		default:
			return nil, fmt.Errorf("line %d: unknown instruction %q", lineNum, parts[0])
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read irollfile: %w", err)
	}

	return lf, nil
}
