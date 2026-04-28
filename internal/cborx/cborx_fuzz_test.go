package cborx

import "testing"

func FuzzUnmarshalSample(f *testing.F) {
	seed, err := Marshal(sample{A: "seed", B: 42})
	if err != nil {
		f.Fatalf("Marshal(seed) error = %v", err)
	}
	f.Add(seed)
	f.Add([]byte{0xa2, 0x61, 0x61, 0x01, 0x61, 0x61, 0x02})
	f.Fuzz(func(t *testing.T, data []byte) {
		var got sample
		_ = Unmarshal(data, &got)
		_ = Wellformed(data)
	})
}
