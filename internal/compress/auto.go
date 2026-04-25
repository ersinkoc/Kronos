package compress

import (
	"bytes"
	"io"
	"math"
)

const (
	autoSampleSize = 1024 * 1024
)

// SelectAlgorithm chooses a compression algorithm from an input sample.
func SelectAlgorithm(sample []byte) Algorithm {
	if len(sample) == 0 {
		return AlgorithmZstd
	}
	entropy := shannonEntropy(sample)
	if entropy > 7.6 {
		return AlgorithmNone
	}
	return AlgorithmZstd
}

// PrepareAuto reads up to 1 MiB, selects an algorithm, and returns a reader
// that includes the sampled bytes followed by the remaining stream.
func PrepareAuto(r io.Reader) (Algorithm, io.Reader, error) {
	var sample bytes.Buffer
	limited := io.LimitReader(r, autoSampleSize)
	if _, err := io.Copy(&sample, limited); err != nil {
		return "", nil, err
	}
	algorithm := SelectAlgorithm(sample.Bytes())
	return algorithm, io.MultiReader(bytes.NewReader(sample.Bytes()), r), nil
}

func shannonEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}
	var counts [256]int
	for _, b := range data {
		counts[b]++
	}
	var entropy float64
	length := float64(len(data))
	for _, count := range counts {
		if count == 0 {
			continue
		}
		p := float64(count) / length
		entropy -= p * math.Log2(p)
	}
	return entropy
}
