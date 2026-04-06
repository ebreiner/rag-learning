package utils

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func PackEmbedding(floatEmbedding []float64) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, floatEmbedding); err != nil {
		return buf, fmt.Errorf("error converting embedding to buffer: %s", err.Error())
	} else {
		return buf, nil
	}
}
