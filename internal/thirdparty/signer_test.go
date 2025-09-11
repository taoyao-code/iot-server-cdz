package thirdparty

import "testing"

func TestSignHMAC(t *testing.T) {
	got := SignHMAC("secret", "METHOD\n/path\n1700000000\nnonce\nbodyhash")
	if len(got) != 64 { // hex-encoded sha256 length
		t.Fatalf("bad length: %s", got)
	}
}
