package batch

import "testing"

func TestParseBatchSeq(t *testing.T) {
	cases := []struct {
		name    string
		id      string
		wantSeq uint64
		wantOK  bool
	}{
		{
			name:    "happy path",
			id:      "batch-1777343491717146300-000001",
			wantSeq: 1,
			wantOK:  true,
		},
		{
			name:    "high seq",
			id:      "batch-1777343491717146300-123456",
			wantSeq: 123456,
			wantOK:  true,
		},
		{
			name:    "seq beyond six digits is still parsed",
			id:      "batch-1777343491717146300-9999999",
			wantSeq: 9999999,
			wantOK:  true,
		},
		{
			name:   "empty",
			id:     "",
			wantOK: false,
		},
		{
			name:   "missing prefix",
			id:     "1777343491717146300-000001",
			wantOK: false,
		},
		{
			name:   "missing seq",
			id:     "batch-1777343491717146300",
			wantOK: false,
		},
		{
			name:   "missing timestamp",
			id:     "batch--000001",
			wantOK: false,
		},
		{
			name:   "trailing dash",
			id:     "batch-1777343491717146300-",
			wantOK: false,
		},
		{
			name:   "non-numeric timestamp",
			id:     "batch-zz-000001",
			wantOK: false,
		},
		{
			name:   "non-numeric seq",
			id:     "batch-1777343491717146300-abc",
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seq, ok := ParseBatchSeq(tc.id)
			if ok != tc.wantOK {
				t.Fatalf("ParseBatchSeq(%q) ok = %v, want %v", tc.id, ok, tc.wantOK)
			}
			if seq != tc.wantSeq {
				t.Fatalf("ParseBatchSeq(%q) seq = %d, want %d", tc.id, seq, tc.wantSeq)
			}
		})
	}
}
