package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/fatih/color"
	uc "github.com/unicorn-engine/unicorn/bindings/go/unicorn"
)

// SHOULD BE GO BUILTIN
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

var steps int = 0
var heap_start uint64 = 0

func WriteBytes(fd int, bytes []byte) {
	printer := color.New(color.FgWhite).SprintFunc()
	if fd == 1 {
		printer = color.New(color.FgGreen).SprintFunc()
	} else if fd == 2 {
		printer = color.New(color.FgRed).SprintFunc()
	}
	os.Stderr.WriteString(printer(string(bytes)))
}

func WriteRam(ram map[uint32](uint32), addr uint32, value uint32) {
	// we no longer delete from ram, since deleting from tries is hard
	if value == 0 && false {
		delete(ram, addr)
	} else {
		/*if addr < 0xc0000000 {
			fmt.Printf("store %x = %x\n", addr, value)
		}*/
		ram[addr] = value
	}
}

var REG_OFFSET uint32 = 0xc0000000
var REG_PC uint32 = REG_OFFSET + 0x20*4
var REG_HEAP uint32 = REG_OFFSET + 0x23*4

func SyncRegs(mu uc.Unicorn, ram map[uint32](uint32)) {
	pc, _ := mu.RegRead(uc.MIPS_REG_PC)
	//fmt.Printf("%d uni %x\n", step, pc)
	WriteRam(ram, 0xc0000080, uint32(pc))

	addr := uint32(0xc0000000)
	for i := uc.MIPS_REG_ZERO; i < uc.MIPS_REG_ZERO+32; i++ {
		reg, _ := mu.RegRead(i)
		WriteRam(ram, addr, uint32(reg))
		addr += 4
	}

	reg_hi, _ := mu.RegRead(uc.MIPS_REG_HI)
	reg_lo, _ := mu.RegRead(uc.MIPS_REG_LO)
	WriteRam(ram, REG_OFFSET+0x21*4, uint32(reg_hi))
	WriteRam(ram, REG_OFFSET+0x22*4, uint32(reg_lo))

	WriteRam(ram, REG_HEAP, uint32(heap_start))
}

func GetHookedUnicorn(root string, ram map[uint32](uint32), callback func(int, uc.Unicorn, map[uint32](uint32))) uc.Unicorn {
	mu, err := uc.NewUnicorn(uc.ARCH_MIPS, uc.MODE_32|uc.MODE_BIG_ENDIAN)
	check(err)

	_, outputfault := os.LookupEnv("OUTPUTFAULT")

	mu.HookAdd(uc.HOOK_INTR, func(mu uc.Unicorn, intno uint32) {
		if intno != 17 {
			log.Fatal("invalid interrupt ", intno, " at step ", steps)
		}
		syscall_no, _ := mu.RegRead(uc.MIPS_REG_V0)
		v0 := uint64(0)
		if syscall_no == 4020 {
			oracle_hash, _ := mu.MemRead(0x30001000, 0x20)
			hash := common.BytesToHash(oracle_hash)
			key := fmt.Sprintf("%s/%s", root, hash)
			value, err := ioutil.ReadFile(key)
			check(err)

			tmp := []byte{0, 0, 0, 0}
			binary.BigEndian.PutUint32(tmp, uint32(len(value)))
			mu.MemWrite(0x31000000, tmp)
			mu.MemWrite(0x31000004, value)

			WriteRam(ram, 0x31000000, uint32(len(value)))
			value = append(value, 0, 0, 0)
			for i := uint32(0); i < ram[0x31000000]; i += 4 {
				WriteRam(ram, 0x31000004+i, binary.BigEndian.Uint32(value[i:i+4]))
			}
		} else if syscall_no == 4004 {
			fd, _ := mu.RegRead(uc.MIPS_REG_A0)
			buf, _ := mu.RegRead(uc.MIPS_REG_A1)
			count, _ := mu.RegRead(uc.MIPS_REG_A2)
			bytes, _ := mu.MemRead(buf, count)
			WriteBytes(int(fd), bytes)
		} else if syscall_no == 4090 {
			a0, _ := mu.RegRead(uc.MIPS_REG_A0)
			sz, _ := mu.RegRead(uc.MIPS_REG_A1)
			if a0 == 0 {
				v0 = 0x20000000 + heap_start
				heap_start += sz
			} else {
				v0 = a0
			}
		} else if syscall_no == 4045 {
			v0 = 0x40000000
		} else if syscall_no == 4120 {
			v0 = 1
		} else if syscall_no == 4246 {
			// exit group
			mu.RegWrite(uc.MIPS_REG_PC, 0x5ead0000)
		} else {
			//fmt.Println("syscall", syscall_no)
		}
		mu.RegWrite(uc.MIPS_REG_V0, v0)
		mu.RegWrite(uc.MIPS_REG_A3, 0)
	}, 0, 0)

	if callback != nil {
		mu.HookAdd(uc.HOOK_MEM_WRITE, func(mu uc.Unicorn, access int, addr64 uint64, size int, value int64) {
			rt := value
			rs := addr64 & 3
			addr := uint32(addr64 & 0xFFFFFFFC)
			if outputfault && addr == 0x30000804 {
				fmt.Printf("injecting output fault over %x\n", rt)
				rt = 0xbabababa
			}
			//fmt.Printf("%X(%d) = %x (at step %d)\n", addr, size, value, steps)
			if size == 1 {
				mem := ram[addr]
				val := uint32((rt & 0xFF) << (24 - (rs&3)*8))
				mask := 0xFFFFFFFF ^ uint32(0xFF<<(24-(rs&3)*8))
				WriteRam(ram, uint32(addr), (mem&mask)|val)
			} else if size == 2 {
				mem := ram[addr]
				val := uint32((rt & 0xFFFF) << (16 - (rs&2)*8))
				mask := 0xFFFFFFFF ^ uint32(0xFFFF<<(16-(rs&2)*8))
				WriteRam(ram, uint32(addr), (mem&mask)|val)
			} else if size == 4 {
				WriteRam(ram, uint32(addr), uint32(rt))
			} else {
				log.Fatal("bad size write to ram")
			}

		}, 0, 0x80000000)

		mu.HookAdd(uc.HOOK_CODE, func(mu uc.Unicorn, addr uint64, size uint32) {
			bs, _ := mu.MemRead(addr, uint64(size))
			inst := binary.BigEndian.Uint32(bs)
			opc := inst >> 26
			if opc == 0 {
				PrintRcode(mu, inst)
			} else if opc == 1 {
				PrintRMMcode(mu, inst)
			} else if opc == 2 || opc == 3 {
				PrintJcode(mu, inst)
			} else if opc == 28 {
				PrintS28code(mu, inst)
			} else if opc > 3 {
				PrintIcode(mu, inst)
			} else {
				fmt.Printf("Opc err:%d,%d\n", opc, inst)
			}
			callback(steps, mu, ram)
			steps += 1
		}, 0, 0x80000000)
	}

	check(mu.MemMap(0, 0x80000000))
	return mu
}

var rmmcode = map[uint32]string{
	0:  "bltz",
	1:  "bgez",
	2:  "bltzl",
	3:  "bgezl",
	17: "bgezal",
}

func PrintRMMcode(mu uc.Unicorn, inst uint32) {
	rs := (inst >> 21) & 0x1f
	fun := (inst >> 16) & 0x1f
	offset := inst & 0x0ffff
	opc := rmmcode[fun]
	switch opc {
	case "bgez", "bgezal", "bltz", "bltzl", "bgezl":
		fmt.Printf("%s %d, %d\n", opc, ReadReg(mu, rs), offset)
	default:
		fmt.Printf("err rmm inst:%d,%d,%s, %d\n", 1, fun, opc, inst)
	}
}

var s28code = map[uint32]string{
	0:  "madd",
	1:  "maddu",
	2:  "mul",
	4:  "msub",
	5:  "msubu",
	32: "clz",
	33: "clo",
}

func PrintS28code(mu uc.Unicorn, inst uint32) {
	fun := inst & 0x3f
	opc := s28code[fun]
	rs := (inst >> 21) & 0x1f
	rt := (inst >> 16) & 0x1f
	rd := (inst >> 11) & 0x1f
	switch opc {
	case "madd", "maddu", "msub", "msubu":
		fmt.Printf("%s %d, %d\n", opc, ReadReg(mu, rs), ReadReg(mu, rt))
	case "mul":
		fmt.Printf("%s %d, %d, %d\n", opc, ReadReg(mu, rd), ReadReg(mu, rs), ReadReg(mu, rt))
	case "clz":
		fmt.Printf("%s %d, %d\n", opc, ReadReg(mu, rd), ReadReg(mu, rs))
	default:
		fmt.Printf("err S28 inst:%d,%d,%s, %d\n", 28, fun, opc, inst)
	}
}

var rcode = map[uint32]string{
	0:  "sll",
	2:  "srl",
	3:  "sra",
	4:  "sllv",
	6:  "srlv",
	7:  "srav",
	8:  "jr",
	9:  "jalr",
	10: "movz",
	11: "movn",
	12: "syscall",
	15: "sync",
	16: "mfhi",
	17: "mthi",
	18: "mflo",
	19: "mtlo",
	24: "mult",
	25: "multu",
	26: "div",
	27: "divu",
	32: "add",
	33: "addu",
	34: "sub",
	35: "subu",
	36: "and",
	37: "or",
	38: "xor",
	39: "nor",
	42: "slt",
	43: "sltu",
}

func PrintRcode(mu uc.Unicorn, inst uint32) {
	fun := inst & 0x3f
	opc := rcode[fun]
	rs := (inst >> 21) & 0x1f
	rt := (inst >> 16) & 0x1f
	rd := (inst >> 11) & 0x1f
	shamt := (inst >> 6) & 0x1f
	switch opc {
	case "sll", "srl", "sra":
		fmt.Printf("%s %d, %d, %d\n", opc, ReadReg(mu, rd), ReadReg(mu, rt), shamt)
	case "sllv", "srlv", "srav":
		fmt.Printf("%s %d, %d, %d\n", opc, ReadReg(mu, rd), ReadReg(mu, rt), ReadReg(mu, rs))
	case "jr":
		fmt.Printf("%s %d\n", opc, ReadReg(mu, rs))
	case "jalr":
		if ReadReg(mu, rd) != 31 {
			fmt.Printf("%s %d, %d\n", opc, ReadReg(mu, rd), ReadReg(mu, rs))
		} else {
			fmt.Printf("%s %d\n", opc, ReadReg(mu, rs))
		}
	case "syscall":
		fmt.Printf("%s\n", opc)
	case "sync":
		fmt.Printf("%s %d\n", opc, shamt)
	case "mfhi", "mflo":
		fmt.Printf("%s %d\n", opc, ReadReg(mu, rd))
	case "mthi":
	case "mtlo":
		fmt.Printf("%s %d\n", opc, ReadReg(mu, rs))
	case "mult", "multu", "div", "divu":
		fmt.Printf("%s %d, %d\n", opc, ReadReg(mu, rs), ReadReg(mu, rt))
	case "add", "addu", "sub", "subu", "and", "or", "xor", "nor", "slt", "sltu", "movz", "movn":
		fmt.Printf("%s %d, %d, %d\n", opc, ReadReg(mu, rd), ReadReg(mu, rs), ReadReg(mu, rt))
	default:
		fmt.Printf("err R inst:%d,%d,%s, %d\n", 0, fun, opc, inst)
	}
}

var jcode = map[uint32]string{
	2: "j",
	3: "jal",
}

func PrintJcode(mu uc.Unicorn, inst uint32) {
	op := inst >> 26
	opc := jcode[op]
	address := inst & 0x03ffffff
	switch opc {
	case "j", "jal":
		fmt.Printf("%s %d\n", opc, address)
	default:
		fmt.Printf("err J inst:%d,%s, %d\n", op, opc, inst)
	}
}

var icode = map[uint32]string{
	4:  "beq",
	5:  "bne",
	6:  "blez",
	7:  "bgtz",
	8:  "addi",
	9:  "addiu",
	10: "slti",
	11: "sltiu",
	12: "andi",
	13: "ori",
	14: "xori",
	15: "lui",
	32: "lb",
	33: "lh",
	34: "lwl",
	35: "lw",
	36: "lbu",
	37: "lhu",
	38: "lwr",
	40: "sb",
	41: "sh",
	42: "swl",
	43: "sw",
	46: "swr",
	48: "ll",
	56: "sc",
}

func PrintIcode(mu uc.Unicorn, inst uint32) {
	op := inst >> 26
	opc := icode[op]
	rs := (inst >> 21) & 0x1f
	rt := (inst >> 16) & 0x1f
	imm := inst & 0x0ffff
	switch opc {
	case "beq", "bne":
		fmt.Printf("%s %d, %d, %d\n", opc, ReadReg(mu, rs), ReadReg(mu, rt), imm)
	case "blez", "bgtz":
		fmt.Printf("%s %d, %d\n", opc, ReadReg(mu, rs), imm)
	case "addi", "addiu", "slti", "sltiu", "andi", "ori", "xori":
		fmt.Printf("%s %d, %d, %d\n", opc, ReadReg(mu, rt), ReadReg(mu, rs), imm)
	case "lui", "lwr", "swl", "swr":
		fmt.Printf("%s %d, %d\n", opc, ReadReg(mu, rt), imm)
	case "lb", "lh", "lwl", "lw", "lbu", "lhu", "sb", "sh", "sw", "ll", "sc":
		fmt.Printf("%s %d, %d (%d)\n", opc, ReadReg(mu, rt), imm, ReadReg(mu, rs))
	default:
		fmt.Printf("err I inst:%d,%s, %d\n", op, opc, inst)
	}

}
func ReadReg(mu uc.Unicorn, rg uint32) uint64 {
	ret, _ := mu.RegRead(int(rg))
	return ret
}

func LoadMappedFileUnicorn(mu uc.Unicorn, fn string, ram map[uint32](uint32), base uint32) {
	dat, err := ioutil.ReadFile(fn)
	check(err)
	LoadData(dat, ram, base)
	mu.MemWrite(uint64(base), dat)
}

// reimplement simple.py in go
func RunUnicorn(fn string, ram map[uint32](uint32), checkIO bool, callback func(int, uc.Unicorn, map[uint32](uint32))) {
	root := "/tmp/cannon/0_13284469"
	mu := GetHookedUnicorn(root, ram, callback)

	// loop forever to match EVM
	//mu.MemMap(0x5ead0000, 0x1000)
	//mu.MemWrite(0xdead0000, []byte{0x08, 0x10, 0x00, 0x00})

	// program
	dat, _ := ioutil.ReadFile(fn)
	mu.MemWrite(0, dat)

	// inputs
	inputs, err := ioutil.ReadFile(fmt.Sprintf("%s/input", root))
	check(err)

	mu.MemWrite(0x30000000, inputs[0:0xc0])

	// load into ram
	LoadData(dat, ram, 0)
	if checkIO {
		LoadData(inputs[0:0xc0], ram, 0x30000000)
	}

	mu.Start(0, 0x5ead0004)

	if checkIO {
		outputs, err := ioutil.ReadFile(fmt.Sprintf("%s/output", root))
		check(err)
		real := append([]byte{0x13, 0x37, 0xf0, 0x0d}, outputs...)
		output, _ := mu.MemRead(0x30000800, 0x44)
		if bytes.Compare(real, output) != 0 {
			log.Fatal("mismatch output")
		} else {
			fmt.Println("output match")
		}
	}
}
