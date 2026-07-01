package main

import (
	"encoding/ascii85"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// Algorithm identifies the text-encoding scheme used before keyboard simulation.
type Algorithm string

const (
	AlgoBase64  Algorithm = "base64"
	AlgoBase85  Algorithm = "base85"
	AlgoBase91  Algorithm = "base91"
	AlgoBase122 Algorithm = "base122"
)

// AlgorithmOverhead is the approximate size ratio (encoded / raw) for display purposes.
var AlgorithmOverhead = map[Algorithm]string{
	AlgoBase64:  "~33%",
	AlgoBase85:  "~25%",
	AlgoBase91:  "~23%",
	AlgoBase122: "~14%",
}

// encodeFile reads inputPath, encodes its contents with algo, and writes to outputPath.
func encodeFile(inputPath, outputPath string, algo Algorithm) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	return os.WriteFile(outputPath, []byte(encodeData(algo, data)), 0644)
}

// encodeData encodes raw bytes using the chosen algorithm.
func encodeData(algo Algorithm, data []byte) string {
	switch algo {
	case AlgoBase85:
		return encodeBase85(data)
	case AlgoBase91:
		return encodeBase91(data)
	case AlgoBase122:
		return encodeBase122(data)
	default:
		return base64.StdEncoding.EncodeToString(data)
	}
}

// ── Base85 (Adobe ascii85) ───────────────────────────────────────────────────
// Uses printable chars 33–117 ('!' through 'u'); 'z' stands for four zero bytes.
// Overhead: ~25 %.

func encodeBase85(data []byte) string {
	dst := make([]byte, ascii85.MaxEncodedLen(len(data)))
	n := ascii85.Encode(dst, data)
	return string(dst[:n])
}

// ── Base91 ───────────────────────────────────────────────────────────────────
// 91-character printable ASCII alphabet.
// Encodes 13 or 14 bits per two-character pair → ~22.8 % overhead.

// 91 printable ASCII chars (A-Z a-z 0-9 + 29 symbols). Count: 26+26+10+29 = 91.
const base91Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!#$%&()*+,./:;<=>?@[]^_`{|}~\""

func encodeBase91(data []byte) string {
	var sb strings.Builder
	b, n := 0, 0
	for _, c := range data {
		b |= int(c) << n
		n += 8
		if n > 13 {
			v := b & 8191 // low 13 bits
			if v > 88 {
				b >>= 13
				n -= 13
			} else {
				v = b & 16383 // low 14 bits
				b >>= 14
				n -= 14
			}
			sb.WriteByte(base91Alphabet[v%91])
			sb.WriteByte(base91Alphabet[v/91])
		}
	}
	if n > 0 {
		sb.WriteByte(base91Alphabet[b%91])
		if n > 7 || b > 90 {
			sb.WriteByte(base91Alphabet[b/91])
		}
	}
	return sb.String()
}

// ── Base122 (Kevin Albertson) ────────────────────────────────────────────────
// Reads 7 bits at a time → emits 1 ASCII byte for "safe" values.
// Six "illegal" values (NUL, LF, CR, ", &, \) are replaced by a 2-byte UTF-8
// sequence (codepoints 128–139) that also absorbs 1 extra input bit.
// Overhead: ~14.3 %.
//
// NOTE: the 2-byte fallback produces non-ASCII runes (codepoints 128–139).
// Typing these via xdotool key U{hex} depends on the target app's Unicode support.

var base122Illegals = [6]byte{0, 10, 13, 34, 38, 92}

func encodeBase122(data []byte) string {
	var sb strings.Builder
	bitBuf, bitsInBuf := 0, 0

	writeChunk := func() {
		v := byte(bitBuf & 0x7F) // consume 7 bits
		bitBuf >>= 7
		bitsInBuf -= 7

		illegalIdx := -1
		for i, ill := range base122Illegals {
			if v == ill {
				illegalIdx = i
				break
			}
		}

		if illegalIdx < 0 {
			sb.WriteByte(v) // safe: 1 UTF-8 byte
			return
		}

		// Unsafe: absorb 1 extra bit and emit a 2-byte UTF-8 codepoint.
		// Codepoints 128–139 each encode (illegal_index, extra_bit) pairs.
		extraBit := 0
		if bitsInBuf > 0 {
			extraBit = bitBuf & 1
			bitBuf >>= 1
			bitsInBuf--
		}
		sb.WriteRune(rune(128 + illegalIdx*2 + extraBit))
	}

	for _, b := range data {
		bitBuf |= int(b) << bitsInBuf
		bitsInBuf += 8
		for bitsInBuf >= 7 {
			writeChunk()
		}
	}
	if bitsInBuf > 0 {
		writeChunk()
	}
	return sb.String()
}
