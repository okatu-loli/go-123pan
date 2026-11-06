package pan123

import "testing"

// Captured successful website requests and a separately executed official-JS
// vector are independent signature oracles. Inputs are timestamps/nonces only;
// these fixtures contain no login credentials or account identifiers.
func TestWebSignatureObservedProtocol(t *testing.T) {
	for _, tc := range []struct {
		seconds int64
		nonce   uint32
		key     string
		value   string
	}{
		{1793954674, 7281101, "1865872004", "1793954674-7281101-1132797017"},
		{1793954881, 2942085, "2503593959", "1793954881-2942085-2537196531"},
		{1793954893, 3824935, "2503593959", "1793954893-3824935-3689354057"},
		{1793952000, 1234567, "3490705463", "1793952000-1234567-3835748752"},
		// Official module at 08:00:59.499Z and 08:00:59.500Z, respectively:
		// the rounded wire second also selects the Beijing minute's key.
		{1793952059, 1234567, "3490705463", "1793952059-1234567-1949791130"},
		{1793952060, 1234567, "2690974904", "1793952060-1234567-689195031"},
	} {
		key, value := webSignature(tc.seconds, tc.nonce, "/b/api/user/info")
		if key != tc.key || value != tc.value {
			t.Fatalf("website signature mismatch for %d", tc.seconds)
		}
	}
}
