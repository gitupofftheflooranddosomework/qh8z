package password

import "testing"

func TestHashVerify(t *testing.T) {
	hash, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !Verify(hash, "correct horse battery staple") {
		t.Fatal("valid password did not verify")
	}
	if Verify(hash, "wrong password") {
		t.Fatal("wrong password verified")
	}
}

func TestValidatePassword(t *testing.T) {
	if err := Validate("too-short"); err == nil {
		t.Fatal("short password accepted")
	}
}
