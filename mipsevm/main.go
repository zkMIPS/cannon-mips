package main

import (
	"bytes"
	"debug/elf"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

var (
	h          bool
	block      string
	program    string
	totalsteps uint
)

func usage() {
	fmt.Fprintf(os.Stderr, `
Usage: mipsevm [-b block] [-e filename] [-s stepNum]

Options:
`)
	flag.PrintDefaults()
}

func init() {
	flag.BoolVar(&h, "h", false, "this help")

	flag.StringVar(&block, "b", "", "blocknum for minigeth")
	flag.StringVar(&program, "e", "", "whole program elf path")
	flag.UintVar(&totalsteps, "s", 0xFFFFFFFF, "program steps")

	// 改变默认的 Usage
	flag.Usage = usage
}

var block_root string

func start_elf(path string) {
	elfProgram, err := elf.Open(path)

	state, err := LoadELF(elfProgram)

	if err != nil {
		fmt.Println(err)
		return
	}

	err = PatchGo(elfProgram, state)

	if err != nil {
		fmt.Println(err)
		return
	}

	err = PatchStack(state)

	if err != nil {
		fmt.Println(err)
		return
	}

	if block != "" {
		block_root = fmt.Sprintf("/tmp/cannon/0_%s", block)
		block_input := fmt.Sprintf("%s/input", block_root)
		state, err = LoadMappedFile(state, block_input, 0x30000000)
	}

	var stdOutBuf, stdErrBuf bytes.Buffer
	goState := NewInstrumentedState(state, nil, io.MultiWriter(&stdOutBuf, os.Stdout), io.MultiWriter(&stdErrBuf, os.Stderr))

	goState.SetBlockRoot(block_root)
	goState.InitialMemRoot()
	//goState.SetDebug(true)

	err = InitDB()

	if err != nil {
		fmt.Println(err)
		return
	}

	start := time.Now()
	step := uint(0)
	for !goState.IsExited() {

		_, err = goState.StepTrace()

		if err != nil {
			fmt.Println(err)
			return
		}

		step++
		if step >= totalsteps {
			break
		}

	}

	fmt.Println("Can ignore ", goState.getIgnoredStep(), " instructions")
	end := time.Now()
	delta := end.Sub(start)
	fmt.Println("test took", delta, ",", state.Step, "instructions, ", delta/time.Duration(state.Step), "per instruction")
}

func start_minigeth() {
	start_elf("minigeth")
	/*
		s := &State{
			PC:        0,
			NextPC:    4,
			HI:        0,
			LO:        0,
			Heap:      0x20000000,
			Registers: [32]uint32{},
			Memory:    NewMemory(),
			ExitCode:  0,
			Exited:    false,
			Step:      0,
		}
		s, err := LoadMappedFile(s, "minigeth.bin", 0)

		if err != nil {
			fmt.Println(err)
			return
		}
		block_root := fmt.Sprintf("/tmp/cannon/0_%s", block)
		block_input := fmt.Sprintf("%s/input", block_root)
		s, err = LoadMappedFile(s, block_input, 0x30000000)

		if err != nil {
			fmt.Println(err)
			return
		}

		var stdOutBuf, stdErrBuf bytes.Buffer
		goState := NewInstrumentedState(s, nil, io.MultiWriter(&stdOutBuf, os.Stdout), io.MultiWriter(&stdErrBuf, os.Stderr))
		goState.SetBlockRoot(block_root)
		goState.SetDebug(true)

		for !goState.IsExited() {
			_, err = goState.StepTrace()

			if err != nil {
				fmt.Println(err)
				return
			}
		}
	*/
}

func main() {

	flag.Parse()

	if h {
		flag.Usage()
	} else if block != "" {
		start_minigeth()
	} else if program != "" {
		start_elf(program)
	}
}
