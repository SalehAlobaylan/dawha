package identity

import "testing"

func TestNormalizeArabicName(t *testing.T) {
	got := NormalizeArabicName("  عَبْدُ   الله، أبو بكر  ")

	if got != "عبد الله ابو بكر" {
		t.Fatalf("unexpected normalized name %q", got)
	}
}

func TestNormalizeArabicNamePreservesMeaningfulLetters(t *testing.T) {
	got := NormalizeArabicName("أحمد بن محمد")

	if got != "احمد بن محمد" {
		t.Fatalf("unexpected normalized name %q", got)
	}
}
