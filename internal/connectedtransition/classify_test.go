package connectedtransition

import (
	"bytes"
	"testing"
)

func TestClassifyStrictRecovery(t *testing.T) {
	for _, operation := range []string{"refresh", "code-select"} {
		r := sampleRecord(operation)
		encoded, err := Encode(r)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := Completion(encoded)
		if err != nil {
			t.Fatal(err)
		}
		for name, resources := range map[string]Resources{
			"previous": r.PreviousResources,
			"next":     r.NextResources,
			"mixed": func() Resources {
				m := r.PreviousResources
				m.CollectorUnit = r.NextResources.CollectorUnit
				return m
			}(),
		} {
			if got, err := Classify(r, Observed{Resources: resources, Ledger: r.Ledger}); err != nil || got != RecoveryRequired {
				t.Fatalf("%s/%s without receipt: %v %v", operation, name, got, err)
			}
			got, err := Classify(r, Observed{Resources: resources, Ledger: r.Ledger, Receipt: receipt})
			if name == "next" {
				if err != nil || got != Complete {
					t.Fatalf("%s complete: %v %v", operation, got, err)
				}
			} else if err == nil {
				t.Fatalf("%s/%s closed over old or mixed files", operation, name)
			}
		}
		actual := Observed{Resources: r.NextResources, Ledger: r.Ledger, Receipt: receipt}
		for name, mutate := range map[string]func(*Observed){
			"CA":            func(o *Observed) { o.Resources.CA = hexByte("f") },
			"hosts":         func(o *Observed) { o.Resources.Hosts = hexByte("f") },
			"collector":     func(o *Observed) { o.Resources.CollectorUnit = hexByte("f") },
			"uploader":      func(o *Observed) { o.Resources.UploaderUnit = hexByte("f") },
			"config":        func(o *Observed) { o.Resources.InstalledConfig = hexByte("f") },
			"ledger":        func(o *Observed) { o.Ledger.Physical.Database.Generation++ },
			"logical":       func(o *Observed) { o.Ledger.LogicalSHA256 = hexByte("f") },
			"receipt":       func(o *Observed) { o.Receipt = bytes.Replace(o.Receipt, []byte("/v1"), []byte("/v2"), 1) },
			"empty receipt": func(o *Observed) { o.Receipt = []byte{} },
		} {
			changed := actual
			mutate(&changed)
			if _, err := Classify(r, changed); err == nil {
				t.Fatalf("%s/%s changed evidence admitted", operation, name)
			}
		}
	}
}
