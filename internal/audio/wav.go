package audio

import (
	"encoding/binary"
	"fmt"
	"io"
)

// ReadPCM16WAV accepts bounded RIFF/WAVE PCM16 mono at the requested rate.
func ReadPCM16WAV(r io.Reader, sampleRate, maxBytes int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(maxBytes+4097)))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBytes+4096 || len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("invalid or oversized WAV")
	}
	if uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data)) {
		return nil, fmt.Errorf("invalid RIFF size")
	}
	validFormat := false
	var pcm []byte
	for offset := 12; offset < len(data); {
		if offset+8 > len(data) {
			return nil, fmt.Errorf("truncated WAV chunk")
		}
		size := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		end := uint64(offset+8) + size
		if end > uint64(len(data)) {
			return nil, fmt.Errorf("truncated WAV payload")
		}
		chunk := data[offset+8 : int(end)]
		switch string(data[offset : offset+4]) {
		case "fmt ":
			if len(chunk) < 16 || binary.LittleEndian.Uint16(chunk[0:2]) != 1 || binary.LittleEndian.Uint16(chunk[2:4]) != 1 || int(binary.LittleEndian.Uint32(chunk[4:8])) != sampleRate || binary.LittleEndian.Uint16(chunk[12:14]) != 2 || binary.LittleEndian.Uint16(chunk[14:16]) != 16 {
				return nil, fmt.Errorf("WAV must be PCM16 mono at %d Hz", sampleRate)
			}
			validFormat = true
		case "data":
			if pcm != nil || len(chunk) == 0 || len(chunk) > maxBytes || len(chunk)%2 != 0 {
				return nil, fmt.Errorf("invalid PCM data")
			}
			pcm = chunk
		}
		if end+size%2 > uint64(len(data)) {
			return nil, fmt.Errorf("missing WAV chunk padding")
		}
		offset = int(end + size%2)
	}
	if !validFormat || pcm == nil {
		return nil, fmt.Errorf("WAV requires fmt and data chunks")
	}
	return pcm, nil
}

func WritePCM16WAV(w io.Writer, pcm []byte, sampleRate int) error {
	if len(pcm) == 0 || len(pcm)%2 != 0 || len(pcm) > 30*48000*2 || sampleRate < 8000 || sampleRate > 48000 {
		return fmt.Errorf("invalid output PCM format or size")
	}
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+len(pcm)))
	copy(header[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(header[32:34], 2)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(len(pcm)))
	if n, err := w.Write(header); err != nil {
		return err
	} else if n != len(header) {
		return io.ErrShortWrite
	}
	if n, err := w.Write(pcm); err != nil {
		return err
	} else if n != len(pcm) {
		return io.ErrShortWrite
	}
	return nil
}
