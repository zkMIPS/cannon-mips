The cannon-mips is a MIPS VM (ported from cannon project) to generate execution records for MIPS programs.

## Directory Layout

```
minigeth -- A standalone "geth" capable of computing a block transition
mipsevm -- A MIPS VM to generate execution records for MIPS program
```

## Building

Pre-requisites: Go, Make.

```
make build
```

## Usage

The following commands should be run from the root directory unless otherwise specified:

```
# compute the transition from 13284469 -> 13284470 on PC
TRANSITION_BLOCK=13284469
mkdir -p /tmp/cannon
minigeth/go-ethereum $TRANSITION_BLOCK

# generate MIPS traces
cd mipsevm
mipsevm -b $TRANSITION_BLOCK
```

## Options for mipsevm

Command: 

```
mipsevm [-h] [-b blocknum] [-e elf-path] [-s stepnum] [-r rate] [-d]
```

Options:

```
  -h             help info
  -b <blocknum>  blocknum for minigeth
  -e <elf-path>  MIPS program elf path(default minigeth when blocknum is specified)
  -s <stepnum>   program steps number to be run (default 4294967295)
  -r <rate>      randomly generate trace rate (1/100000) (default 100000)
  -d             enable debug output for the instrution sequences
```

## License

Most of this code is MIT licensed, minigeth is LGPL3.

Note: This code is unaudited. It in NO WAY should be used to secure any money until a lot more
testing and auditing are done. I have deployed this nowhere, have advised against deploying it, and
make no guarantees of security of ANY KIND.
