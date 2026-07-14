package privacy

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestProtectorEncryptDecryptLookupAndRotation(t *testing.T) {
	keyV1 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	keyV2 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	lookup := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	protector, err := NewProtector("v2", map[string]string{"v1": keyV1, "v2": keyV2}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	aad := []byte("users:mobile:1001")
	first, version, err := protector.Encrypt("13800138000", aad)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := protector.Encrypt("13800138000", aad)
	if err != nil {
		t.Fatal(err)
	}
	if version != "v2" || bytes.Equal(first, second) {
		t.Fatalf("version=%s ciphertexts must use random nonces", version)
	}
	plaintext, err := protector.Decrypt(first, version, aad)
	if err != nil || plaintext != "13800138000" {
		t.Fatalf("plaintext=%q err=%v", plaintext, err)
	}
	if _, err := protector.Decrypt(first, version, []byte("users:mobile:other")); err == nil {
		t.Fatal("expected additional data mismatch to fail authentication")
	}
	if !bytes.Equal(protector.Lookup("13800138000"), protector.Lookup("13800138000")) {
		t.Fatal("blind index must be deterministic")
	}
}

func TestMaskPrivateFields(t *testing.T) {
	tests := map[string]struct {
		got  string
		want string
	}{
		"mobile":      {MaskMobile("13800138000"), "138****8000"},
		"certificate": {MaskCertificate("310101199001010011"), "3101**********0011"},
		"name":        {MaskName("张三丰"), "张**"},
		"email":       {MaskEmail("Ticket@example.com"), "ti****@example.com"},
	}
	for name, test := range tests {
		if test.got != test.want {
			t.Fatalf("%s got=%q want=%q", name, test.got, test.want)
		}
	}
}
