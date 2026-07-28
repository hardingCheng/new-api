package common

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetM4ADurationRejectsZeroTimescale(t *testing.T) {
	mvhdPayload := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhdPayload[16:20], 1000)

	data := make([]byte, 116)
	binary.BigEndian.PutUint32(data[0:4], 116)
	copy(data[4:8], "moov")
	binary.BigEndian.PutUint32(data[8:12], 108)
	copy(data[12:16], "mvhd")
	copy(data[16:], mvhdPayload)

	_, err := getM4ADuration(bytes.NewReader(data))
	require.ErrorContains(t, err, "timescale")
}
