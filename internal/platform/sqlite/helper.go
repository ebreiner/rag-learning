package sqlite

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func PackVector(vec []float64) ([]byte, error) {
	narrowed := make([]float32, len(vec))
	for i, v := range vec {
		narrowed[i] = float32(v)
	}
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, narrowed); err != nil {
		return nil, fmt.Errorf("packing vector: %w", err)
	}
	return buf.Bytes(), nil
}
