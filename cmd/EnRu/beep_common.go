package main

import (
	"encoding/binary"
	"os"
)

// generateWAV генерирует WAV-данные для тона заданной частоты и длительности
func generateWAV(freq uint, durationMs int) []byte {
	sampleRate := uint32(44100)
	numSamples := sampleRate * uint32(durationMs) / 1000
	numChannels := uint16(1)
	bitsPerSample := uint16(16)
	byteRate := sampleRate * uint32(numChannels) * uint32(bitsPerSample) / 8
	blockAlign := uint16(numChannels) * bitsPerSample / 8
	dataSize := numSamples * uint32(blockAlign)
	fileSize := uint32(36) + dataSize

	buf := make([]byte, 44+dataSize)

	// RIFF header
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], fileSize)
	copy(buf[8:12], "WAVE")

	// fmt chunk
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16) // chunk size
	binary.LittleEndian.PutUint16(buf[20:22], 1)  // PCM
	binary.LittleEndian.PutUint16(buf[22:24], numChannels)
	binary.LittleEndian.PutUint32(buf[24:28], sampleRate)
	binary.LittleEndian.PutUint32(buf[28:32], byteRate)
	binary.LittleEndian.PutUint16(buf[32:34], blockAlign)
	binary.LittleEndian.PutUint16(buf[34:36], bitsPerSample)

	// data chunk
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], dataSize)

	// Генерация синусоиды
	amplitude := float64(32767 * 0.3) // 30% громкости
	for i := uint32(0); i < numSamples; i++ {
		t := float64(i) / float64(sampleRate)
		sample := int16(amplitude * sin(2*3.14159265358979323846*float64(freq)*t))
		binary.LittleEndian.PutUint16(buf[44+i*2:], uint16(sample))
	}

	return buf
}

func sin(x float64) float64 {
	pi := 3.14159265358979323846
	x = x - float64(int(x/(2*pi)))*2*pi
	if x < 0 {
		x = -x
	}
	if x > pi {
		x = x - pi
		return -x * (1.2732395447351627 - 0.40528473456935109*x)
	}
	return x * (1.2732395447351627 - 0.40528473456935109*x)
}

func generateWAVFile(freq uint, durationMs int) (string, error) {
	data := generateWAV(freq, durationMs)
	tmpFile, err := os.CreateTemp("", "enru-beep-*.wav")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()
	_, err = tmpFile.Write(data)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}
	return tmpFile.Name(), nil
}
