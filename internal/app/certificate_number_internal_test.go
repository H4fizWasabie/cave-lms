package app

import "testing"

func TestACertificateNoWidensPastTheYearsNineThousandNineHundredAndNinetyNinth(t *testing.T) {
	for seq, want := range map[int]string{1: "PCC-LMS-260001", 9999: "PCC-LMS-269999", 10000: "PCC-LMS-2610000"} {
		if got := (certificate{Year: 2026, Seq: seq}).Number(); got != want {
			t.Errorf("the 2026 Certificate %d is numbered %s, want %s", seq, got, want)
		}
	}
}
