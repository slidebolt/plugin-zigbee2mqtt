package translate

import (
	"encoding/json"
	"testing"
)

func TestDiscoveryPayload_DecodesFieldsThatShareAKey(t *testing.T) {
	var number DiscoveryPayload
	if err := json.Unmarshal([]byte(`{"min":0.5,"max":10.5,"step":0.5}`), &number); err != nil {
		t.Fatal(err)
	}
	if number.Min != 0.5 || number.Max != 10.5 {
		t.Errorf("number range = %v..%v, want 0.5..10.5", number.Min, number.Max)
	}

	var text DiscoveryPayload
	if err := json.Unmarshal([]byte(`{"min":2,"max":20}`), &text); err != nil {
		t.Fatal(err)
	}
	if text.TextMin != 2 || text.TextMax != 20 {
		t.Errorf("text length = %d..%d, want 2..20", text.TextMin, text.TextMax)
	}

	var climate DiscoveryPayload
	if err := json.Unmarshal([]byte(`{"temp_step":0.5,"precision":0.1}`), &climate); err != nil {
		t.Fatal(err)
	}
	if climate.TempStep != 0.5 || climate.Precision != 0.1 {
		t.Errorf("temp_step = %v, precision = %v, want 0.5, 0.1", climate.TempStep, climate.Precision)
	}
}
