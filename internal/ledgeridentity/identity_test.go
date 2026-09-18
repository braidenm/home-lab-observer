package ledgeridentity

import "testing"

func validWitness() Witness {
	return Witness{"0123456789abcdef0123456789abcdef", Object{12, 34}, Object{56, 78}}
}

func TestWitnessValidation(t *testing.T) {
	if Validate(validWitness()) != nil {
		t.Fatal("synthetic witness refused")
	}
	for _, change := range []func(*Witness){
		func(w *Witness) { w.FilesystemUUID = "" },
		func(w *Witness) { w.FilesystemUUID = "0123456789ABCDEF0123456789abcdef" },
		func(w *Witness) { w.FilesystemUUID = "00000000000000000000000000000000" },
		func(w *Witness) { w.FilesystemUUID = "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz" },
		func(w *Witness) { w.Directory.Inode = 0 }, func(w *Witness) { w.Database.Inode = 0 },
		func(w *Witness) { w.Directory.Generation = 0 }, func(w *Witness) { w.Database.Generation = 0 },
		func(w *Witness) { w.Database.Inode = w.Directory.Inode },
	} {
		w := validWitness()
		change(&w)
		if Validate(w) != ErrUnavailable {
			t.Fatal("invalid witness accepted")
		}
	}
}
