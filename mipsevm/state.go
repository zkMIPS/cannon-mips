package main

import (
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"log"
	"strconv"

	_ "github.com/lib/pq"
	//"database/sql"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type State struct {
	Memory *Memory `json:"memory"`

	PreimageKey    common.Hash `json:"preimageKey"`
	PreimageOffset uint32      `json:"preimageOffset"` // note that the offset includes the 8-byte length prefix

	PC     uint32 `json:"pc"`
	NextPC uint32 `json:"nextPC"`
	LO     uint32 `json:"lo"`
	HI     uint32 `json:"hi"`
	Heap   uint32 `json:"heap"` // to handle mmap growth

	ExitCode uint8 `json:"exit"`
	Exited   bool  `json:"exited"`

	Step uint64 `json:"step"`

	Registers [32]uint32 `json:"registers"`

	// LastHint is optional metadata, and not part of the VM state itself.
	// It is used to remember the last pre-image hint,
	// so a VM can start from any state without fetching prior pre-images,
	// and instead just repeat the last hint on setup,
	// to make sure pre-image requests can be served.
	// The first 4 bytes are a uin32 length prefix.
	// Warning: the hint MAY NOT BE COMPLETE. I.e. this is buffered,
	// and should only be read when len(LastHint) > 4 && uint32(LastHint[:4]) >= len(LastHint[4:])
	LastHint hexutil.Bytes `json:"lastHint,omitempty"`
}

type traceState struct {
	Step   uint64 `json:"cycle"`
	PC     uint32 `json:"pc"`
	NextPC uint32 `json:"nextPC"`

	LO uint32 `json:"lo"`
	HI uint32 `json:"hi"`

	Registers [32]uint32 `json:"regs"`

	//PreimageKey   [32]byte `json:"preimageKey"`
	//PreimageOffset uint32      `json:"preimageOffset"` // note that the offset includes the 8-byte length prefix

	Heap uint32 `json:"heap"` // to handle mmap growth

	ExitCode uint8     `json:"exitCode"`
	Exited   bool      `json:"exited"`
	MemRoot  [32]uint8 `json:"memRoot"`

	Insn_proof   [28 * 32]uint8 `json:"insn_proof"`
	Memory_proof [28 * 32]uint8 `json:"mem_proof"`
}

type traceStateJson struct {
	Step   string `json:"cycle"`
	PC     string `json:"pc"`
	NextPC string `json:"nextPC"`

	LO string `json:"lo"`
	HI string `json:"hi"`

	Registers [32]string `json:"regs"`

	//PreimageKey   [32]byte `json:"preimageKey"`
	//PreimageOffset uint32      `json:"preimageOffset"` // note that the offset includes the 8-byte length prefix

	Heap string `json:"heap"` // to handle mmap growth

	ExitCode string     `json:"exitCode"`
	Exited   bool       `json:"exited"`
	MemRoot  [32]string `json:"memRoot"`

	Insn_proof   [28 * 32]string `json:"insn_proof"`
	Memory_proof [28 * 32]string `json:"mem_proof"`
}

var (
	DB *sql.DB
)

func (a traceStateJson) Value() (driver.Value, error) {
	return json.Marshal(a)
}

// Make the Attrs struct implement the sql.Scanner interface. This method
// simply decodes a JSON-encoded value into the struct fields.
func (a *traceStateJson) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &a)
}

func InitDB() (err error) {
	// ec2-46-51-227-198.ap-northeast-1.compute.amazonaws.com
	db, err := sql.Open("postgres", "sslmode=disable user=postgres password=mipszero host=46.51.227.198 port=5432 dbname=zkmips")
	if err != nil {
		return err
	}

	_, err = db.Exec("TRUNCATE f_traces")

	if err != nil {
		return err
	}

	DB = db
	return nil
}

func (s *traceState) insertToDB() {
	json := &traceStateJson{
		Step:   strconv.FormatUint(uint64(s.Step), 10),
		PC:     strconv.FormatUint(uint64(s.PC), 10),
		NextPC: strconv.FormatUint(uint64(s.NextPC), 10),

		LO:   strconv.FormatUint(uint64(s.LO), 10),
		HI:   strconv.FormatUint(uint64(s.HI), 10),
		Heap: strconv.FormatUint(uint64(s.Heap), 10),

		ExitCode: strconv.FormatUint(uint64(s.ExitCode), 10),
		Exited:   s.Exited,
	}

	for i := int(0); i < 32; i++ {
		json.MemRoot[i] = strconv.FormatUint(uint64(s.MemRoot[i]), 10)
		json.Registers[i] = strconv.FormatUint(uint64(s.Registers[i]), 10)
	}

	for i := int(0); i < 32*28; i++ {
		json.Insn_proof[i] = strconv.FormatUint(uint64(s.Insn_proof[i]), 10)
		json.Memory_proof[i] = strconv.FormatUint(uint64(s.Memory_proof[i]), 10)
	}

	_, err := DB.Exec("INSERT INTO f_traces (f_trace) VALUES($1)", json)

	if err != nil {
		log.Fatal(err)
	}

}

func (s *State) EncodeWitness() []byte {
	out := make([]byte, 0)
	memRoot := s.Memory.MerkleRoot()
	out = append(out, memRoot[:]...)
	out = append(out, s.PreimageKey[:]...)
	out = binary.BigEndian.AppendUint32(out, s.PreimageOffset)
	out = binary.BigEndian.AppendUint32(out, s.PC)
	out = binary.BigEndian.AppendUint32(out, s.NextPC)
	out = binary.BigEndian.AppendUint32(out, s.LO)
	out = binary.BigEndian.AppendUint32(out, s.HI)
	out = binary.BigEndian.AppendUint32(out, s.Heap)
	out = append(out, s.ExitCode)
	if s.Exited {
		out = append(out, 1)
	} else {
		out = append(out, 0)
	}
	out = binary.BigEndian.AppendUint64(out, s.Step)
	for _, r := range s.Registers {
		out = binary.BigEndian.AppendUint32(out, r)
	}
	return out
}
