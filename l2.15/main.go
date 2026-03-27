package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type Command struct {
	Args []string
}

type Pipeline struct {
	Cmds []Command
}

type Shell struct {
	in  io.Reader
	out io.Writer
	err io.Writer

	mu       sync.Mutex
	running  []*exec.Cmd
	sigCh    chan os.Signal
	shutdown chan struct{}
}

func NewShell(in io.Reader, out, err io.Writer) *Shell {
	sh := &Shell{
		in:       in,
		out:      out,
		err:      err,
		sigCh:    make(chan os.Signal, 1),
		shutdown: make(chan struct{}),
	}
	signal.Notify(sh.sigCh, os.Interrupt)
	go sh.signalLoop()
	return sh
}

func (s *Shell) Close() {
	close(s.shutdown)
	signal.Stop(s.sigCh)
}

func (s *Shell) signalLoop() {
	for {
		select {
		case <-s.shutdown:
			return
		case <-s.sigCh:
			s.killRunning()
		}
	}
}

func (s *Shell) setRunning(cmds []*exec.Cmd) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = cmds
}

func (s *Shell) clearRunning() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = nil
}

func (s *Shell) killRunning() {
	s.mu.Lock()
	cmds := append([]*exec.Cmd(nil), s.running...)
	s.mu.Unlock()

	for _, c := range cmds {
		if c == nil || c.Process == nil {
			continue
		}
		_ = c.Process.Kill()
	}
}

func (s *Shell) Run() int {
	reader := bufio.NewReader(s.in)

	for {
		fmt.Fprint(s.out, "minish> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if strings.TrimSpace(line) == "" {
					fmt.Fprintln(s.out)
					return 0
				}
			} else {
				fmt.Fprintln(s.err, "read error:", err)
				continue
			}
		}

		line = strings.TrimSpace(line)
		if line == "" {
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(s.out)
				return 0
			}
			continue
		}

		pl, perr := parsePipeline(line)
		if perr != nil {
			fmt.Fprintln(s.err, "parse error:", perr)
			continue
		}

		if len(pl.Cmds) == 1 && len(pl.Cmds[0].Args) > 0 {
			if s.tryBuiltin(pl.Cmds[0].Args) {
				continue
			}
		}

		if err := s.execPipeline(pl); err != nil {
			fmt.Fprintln(s.err, err.Error())
		}

		if errors.Is(err, io.EOF) {
			fmt.Fprintln(s.out)
			return 0
		}
	}
}

func parsePipeline(line string) (Pipeline, error) {
	parts := strings.Split(line, "|") 
	if len(parts) == 0 {
		return Pipeline{}, fmt.Errorf("empty command")
	}

	var cmds []Command
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return Pipeline{}, fmt.Errorf("empty pipeline segment")
		}
		args := strings.Fields(p)
		if len(args) == 0 {
			return Pipeline{}, fmt.Errorf("empty pipeline segment")
		}
		cmds = append(cmds, Command{Args: args})
	}
	return Pipeline{Cmds: cmds}, nil
}

func (s *Shell) tryBuiltin(args []string) bool {
	switch args[0] {
	case "cd":
		s.builtinCD(args[1:])
		return true
	case "pwd":
		s.builtinPWD()
		return true
	case "echo":
		s.builtinEcho(args[1:])
		return true
	case "kill":
		s.builtinKill(args[1:])
		return true
	case "ps":
		s.builtinPS()
		return true
	case "exit":
		os.Exit(0)
		return true
	default:
		return false
	}
}

func (s *Shell) builtinCD(args []string) {
	var target string
	if len(args) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(s.err, "cd:", err)
			return
		}
		target = home
	} else {
		target = args[0]
	}
	if err := os.Chdir(target); err != nil {
		fmt.Fprintln(s.err, "cd:", err)
	}
}

func (s *Shell) builtinPWD() {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(s.err, "pwd:", err)
		return
	}
	fmt.Fprintln(s.out, wd)
}

func (s *Shell) builtinEcho(args []string) {
	fmt.Fprintln(s.out, strings.Join(args, " "))
}

func (s *Shell) builtinKill(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(s.err, "kill: usage: kill <pid>")
		return
	}
	pid, err := strconv.Atoi(args[0])
	if err != nil || pid <= 0 {
		fmt.Fprintln(s.err, "kill: invalid pid")
		return
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintln(s.err, "kill:", err)
		return
	}
	if err := p.Kill(); err != nil {
		fmt.Fprintln(s.err, "kill:", err)
	}
}

func (s *Shell) builtinPS() {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("tasklist")
		cmd.Stdout = s.out
		cmd.Stderr = s.err
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(s.err, "ps:", err)
		}
		return
	}

	cmd := exec.Command("ps", "aux")
	cmd.Stdout = s.out
	cmd.Stderr = s.err
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(s.err, "ps:", err)
	}
}

func (s *Shell) execPipeline(pl Pipeline) error {
	cmds := make([]*exec.Cmd, 0, len(pl.Cmds))
	for _, c := range pl.Cmds {
		cmds = append(cmds, buildExecCmd(c.Args))
	}
	var (
		pipeReaders []*os.File
		pipeWriters []*os.File
	)
	for i := 0; i < len(cmds)-1; i++ {
		r, w, err := os.Pipe()
		if err != nil {
			for _, rr := range pipeReaders {
				_ = rr.Close()
			}
			for _, ww := range pipeWriters {
				_ = ww.Close()
			}
			return fmt.Errorf("pipe: %v", err)
		}
		pipeReaders = append(pipeReaders, r)
		pipeWriters = append(pipeWriters, w)
	}

	for i, c := range cmds {
		if i == 0 {
			c.Stdin = s.in
		} else {
			c.Stdin = pipeReaders[i-1]
		}
		if i == len(cmds)-1 {
			c.Stdout = s.out
		} else {
			c.Stdout = pipeWriters[i]
		}
		c.Stderr = s.err
	}

	s.setRunning(cmds)
	defer s.clearRunning()

	for i, c := range cmds {
		if err := c.Start(); err != nil {
			for _, rr := range pipeReaders {
				_ = rr.Close()
			}
			for _, ww := range pipeWriters {
				_ = ww.Close()
			}
			for j := 0; j < i; j++ {
				if cmds[j].Process != nil {
					_ = cmds[j].Process.Kill()
				}
			}
			return fmt.Errorf("%s: %v", strings.Join(pl.Cmds[i].Args, " "), err)
		}
	}

	for _, rr := range pipeReaders {
		_ = rr.Close()
	}
	for _, ww := range pipeWriters {
		_ = ww.Close()
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(cmds))
	for i, c := range cmds {
		wg.Add(1)
		go func(i int, c *exec.Cmd) {
			defer wg.Done()
			if err := c.Wait(); err != nil {
				errCh <- fmt.Errorf("%s: %v", strings.Join(pl.Cmds[i].Args, " "), err)
			}
		}(i, c)
	}
	wg.Wait()
	close(errCh)

	for e := range errCh {
		return e
	}
	return nil
}

func buildExecCmd(args []string) *exec.Cmd {
	if runtime.GOOS == "windows" && len(args) > 0 && args[0] == "ps" {
		args = []string{"tasklist"}
	}

	cmd := exec.Command(args[0], args[1:]...)
	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = wd
	}
	return cmd
}

func main() {
	if wd, err := os.Getwd(); err == nil {
		_ = os.Chdir(filepath.Clean(wd))
	}
	sh := NewShell(os.Stdin, os.Stdout, os.Stderr)
	defer sh.Close()
	os.Exit(sh.Run())
}

