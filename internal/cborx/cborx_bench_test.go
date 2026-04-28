package cborx

import "testing"

func BenchmarkMarshalSmallStruct(b *testing.B) {
	v := sample{A: "client-a", B: 42}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Marshal(v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalSmallStruct(b *testing.B) {
	data, err := Marshal(sample{A: "client-a", B: 42})
	if err != nil {
		b.Fatal(err)
	}
	var v sample
	b.ReportAllocs()
	for b.Loop() {
		v = sample{}
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}
