package capture

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRejectIneligibleEvidence(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{"id":1,"id":2}`), []byte(`{"x":{"id":1,"id":2}}`),
		[]byte(`{} {}`), []byte(`{"x":`), []byte(`<html>error</html>`),
		{'"', 0xff, '"'}, []byte(strings.Repeat("[", MaxDepth+1) + "0" + strings.Repeat("]", MaxDepth+1)),
		[]byte("[" + strings.Repeat("0,", MaxNodes) + "0]"), []byte(`NaN`),
		[]byte(`"` + strings.Repeat("x", MaxBodyBytes) + `"`),
	} {
		if _, err := Decode(body); err == nil {
			t.Fatalf("accepted ineligible body of length %d", len(body))
		}
	}
}

func TestExactNumberAndMissingNull(t *testing.T) {
	value, err := Decode([]byte(`{"id":9007199254740993,"amount":0.0100,"note":null}`))
	if err != nil {
		t.Fatal(err)
	}
	obj := value.(map[string]any)
	if obj["id"].(json.Number).String() != "9007199254740993" || obj["amount"].(json.Number).String() != "0.0100" {
		t.Fatal("number token changed")
	}
	if _, present := obj["note"]; !present {
		t.Fatal("null became missing")
	}
	if _, present := obj["missing"]; present {
		t.Fatal("missing field appeared")
	}
}

func TestRedactionDoesNotLeakKeysOrScalarSecrets(t *testing.T) {
	value, err := Decode([]byte(`{"SECRET_KEY":{"value":"SECRET_VALUE","token":123456789,"flag":true},"note":null}`))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(Redact(value))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRET_KEY", "SECRET_VALUE", "123456789", "true", "token"} {
		if strings.Contains(string(body), secret) {
			t.Fatal("secret exported")
		}
	}
	if !strings.Contains(string(body), `"kind":"null"`) {
		t.Fatal("null shape missing")
	}
}
