#!/usr/bin/env bash

# The following variables can be overridden as environment variables:
# * BLOCK (block whose transition will be challenged)
# * SKIP_NODE (skip forking a node, useful if you've already forked a node)
#
# Example usage:
# SKIP_NODE=1 BLOCK=13284469 WRONG_BLOCK=13284491 ./demo/challenge_simple.sh

# --- DOC ----------------------------------------------------------------------

# In this example, the challenger will challenge the transition from a block
# (`BLOCK`), but pretends that chain state before another block (`WRONG_BLOCK`)
# is the state before the challenged block. Consequently, the challenger will
# disagree with the defender on every single step of the challenge game, and the
# single step to execute will be the very first MIPS instruction executed. The
# reason is that the initial MIPS state Merkle root is stored on-chain, and
# immediately modified to reflect the fact that the input hash for the block is
# written at address 0x3000000.
#
# (The input hash is automatically validated against the blockhash, so note that
# in this demo the challenger has to provide the correct (`BLOCK`) input hash to
# the `initiateChallenge` function of `Challenge.sol`, but will execute as
# though the input hash was the one derived from `WRONG_BLOCK`.)
#
# Because the challenger uses the wrong inputs, it will assert a post-state
# (Merkle root) for the first MIPS instruction that has the wrong input hash at
# 0x3000000. Hence, the challenge will fail.


# --- SCRIPT SETUP -------------------------------------------------------------

shout() {
    echo ""
    echo "----------------------------------------"
    echo "$1"
    echo "----------------------------------------"
    echo ""
}

# Exit if any command fails.
set -e

exit_trap() {
    # Print an error if the last command failed
    # (in which case the script is exiting because of set -e).
    [[ $? == 0 ]] && return
    echo "----------------------------------------"
    echo "EARLY EXIT: SCRIPT FAILED"
    echo "----------------------------------------"

    # Kill (send SIGTERM) to the whole process group, also killing
    # any background processes.
    # I think the trap command resets SIGTERM before resending it to the whole
    # group. (cf. https://stackoverflow.com/a/2173421)
    trap - SIGTERM && kill -- -$$
}
trap "exit_trap" SIGINT SIGTERM EXIT

# --- BOOT MAINNET FORK --------------------------------------------------------

if [[ ! "$SKIP_NODE" ]]; then
    NODE_LOG="challenge_simple_node.log"

    shout "BOOTING MAINNET FORK NODE IN BACKGROUND (LOG: $NODE_LOG)"

    # get directory containing this file
    SCRIPT_DIR=$(dirname "$(readlink -f "$0")")

    # run a hardhat mainnet fork node
    "$SCRIPT_DIR/forked_node.sh" > "$NODE_LOG" 2>&1 &

    # give the node some time to boot up
    sleep 10
fi

# --- CHALLENGE SETUP ----------------------------------------------------------

# hardhat network to use
NETWORK=${NETWORK:-l1}
export NETWORK

# challenge ID, read by respond.js and assert.js
export ID=0

# block whose transition will be challenged
# this variable is read by challenge.js, respond.js and assert.js
BLOCK=${BLOCK:-13284469}
export BLOCK

# clear data from previous runs
mkdir -p /tmp/cannon /tmp/cannon_fault && rm -rf /tmp/cannon/* /tmp/cannon_fault/*

# stored in /tmp/cannon/golden.json
shout "GENERATING INITIAL MEMORY STATE CHECKPOINT"
mipsevm/mipsevm

# challenger will use same initial memory checkpoint and deployed contracts
cp /tmp/cannon/golden.json /tmp/cannon_fault/

shout "FETCHING PREIMAGES FOR REAL BLOCK"
minigeth/go-ethereum $BLOCK

shout "COMPUTING REAL MIPS FINAL MEMORY CHECKPOINT"
mipsevm/mipsevm $BLOCK

