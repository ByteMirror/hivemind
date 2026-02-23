package memory

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCosine_IdenticalVectors(t *testing.T) {
	v := []float32{1, 2, 3}
	score := cosine(v, v)
	assert.InDelta(t, 1.0, score, 0.001)
}

func TestCosine_OrthogonalVectors(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	score := cosine(a, b)
	assert.InDelta(t, 0.0, score, 0.001)
}

func TestCosine_KnownValue(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{1, 1}
	// cos(45°) = 1/sqrt(2) ≈ 0.707
	score := cosine(a, b)
	assert.InDelta(t, 1.0/math.Sqrt2, float64(score), 0.001)
}

func TestSerializeRoundtrip(t *testing.T) {
	v := []float32{1.5, 2.5, -3.0, 0.0}
	blob := serializeVec(v)
	got := deserializeVec(blob)
	assert.Equal(t, v, got)
}
