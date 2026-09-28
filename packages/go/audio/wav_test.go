package audio

import (
	"bytes"
	"testing"
)

func TestWAVRoundTripAndLimits(t *testing.T) {
	var output bytes.Buffer
	pcm := []byte{1, 2, 3, 4}
	if err := WritePCM16WAV(&output, pcm, 16000); err != nil {
		t.Fatal(err)
	}
	read, err := ReadPCM16WAV(bytes.NewReader(output.Bytes()), 16000, 4)
	if err != nil || !bytes.Equal(read, pcm) {
		t.Fatal(read, err)
	}
	if _, err := ReadPCM16WAV(bytes.NewReader(output.Bytes()), 24000, 4); err == nil {
		t.Fatal("accepted wrong sample rate")
	}
	if _, err := ReadPCM16WAV(bytes.NewReader(output.Bytes()), 16000, 2); err == nil {
		t.Fatal("accepted oversized data")
	}
	if _, err := ReadPCM16WAV(bytes.NewReader(output.Bytes()[:20]), 16000, 4); err == nil {
		t.Fatal("accepted truncated file")
	}
	if err := WritePCM16WAV(&output, []byte{1}, 16000); err == nil {
		t.Fatal("accepted odd PCM")
	}
}
