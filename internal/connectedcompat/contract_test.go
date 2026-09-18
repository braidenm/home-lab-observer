package connectedcompat_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedcompat"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

func TestCodeOwnedDescriptor(t *testing.T) {
	const golden = "8ad68a7396f61460e50fc59a59fa952089ac83580d713c5684f985ec47845f56"
	data := connectedcompat.Descriptor()
	var compact bytes.Buffer
	if len(data) > 16384 || json.Compact(&compact, data) != nil || !bytes.Equal(append(compact.Bytes(), '\n'), data) || connectedcompat.Digest() != golden || !connectedcompat.Known(golden) {
		t.Fatal("descriptor bytes changed without explicit contract review")
	}
	for _, bad := range []string{"", strings.Repeat("a", 64), strings.ToUpper(golden), golden + "\n"} {
		if connectedcompat.Known(bad) {
			t.Fatal("caller supplied contract accepted")
		}
	}
	data[0] = 'x'
	if connectedcompat.Descriptor()[0] != '{' || connectedcompat.Digest() != golden {
		t.Fatal("descriptor authority aliases caller buffer")
	}
}

func TestDescriptorMatchesResourcesAndState(t *testing.T) {
	var d struct {
		Ledger struct {
			Schema string `json:"schema_sha256"`
		} `json:"ledger"`
		State struct {
			Age     int `json:"max_age_seconds"`
			Future  int `json:"max_future_seconds"`
			Timeout int `json:"request_timeout_seconds"`
		} `json:"upload_state"`
		Wire struct {
			Admitted       []string `json:"admitted"`
			Producer       string   `json:"producer"`
			MaxBytes       int      `json:"max_bytes"`
			MaxFilesystems int      `json:"max_filesystems"`
			MaxInteger     uint64   `json:"max_exact_integer"`
		} `json:"wire"`
		Resources  map[string]string `json:"resources"`
		Transition string            `json:"transition_protocol"`
		Filesystem string            `json:"durable_filesystem_identity"`
	}
	if json.Unmarshal(connectedcompat.Descriptor(), &d) != nil {
		t.Fatal("descriptor")
	}
	if time.Duration(d.State.Age)*time.Second != uploadstate.MaxAge || time.Duration(d.State.Future)*time.Second != uploadstate.MaxFuture || time.Duration(d.State.Timeout)*time.Second != uploadstate.RequestTimeout || d.Wire.MaxBytes != remoteprojection.MaxBytes || d.Wire.MaxFilesystems != remoteprojection.MaxFilesystems || d.Wire.MaxInteger != remoteprojection.MaxExactInteger || d.Wire.Producer != remoteprojection.NativeSchemaVersion || len(d.Wire.Admitted) != 2 || d.Wire.Admitted[0] != "home-lab-server-snapshot/v1:numeric-host/v1" || d.Wire.Admitted[1] != remoteprojection.NativeSchemaVersion {
		t.Fatal("current wire/state contract differs")
	}
	if d.Transition != "not-implemented" || d.Filesystem != "not-asserted" {
		t.Fatal("unproved transition capability asserted")
	}
	resources := connectedunits.Resources()
	if !connectedcompat.MatchesResources(resources) {
		t.Fatal("compiled resources differ from descriptor")
	}
	if len(resources) != len(d.Resources) {
		t.Fatal("resource set changed")
	}
	for name, data := range resources {
		sum := sha256.Sum256(data)
		if d.Resources[name] != hex.EncodeToString(sum[:]) {
			t.Fatal("reviewed resource contract differs", name)
		}
	}
	resources["enrollment.properties.tmpl"] = []byte("changed")
	if connectedcompat.MatchesResources(resources) {
		t.Fatal("changed compiled resources accepted")
	}
	// Parse the exact Go raw-string SQL constant without importing Linux storage
	// into worker identity. A schema change must deliberately revise the contract.
	f, err := parser.ParseFile(token.NewFileSet(), "../uploadledger/schema_linux.go", nil, 0)
	if err != nil {
		t.Fatal("schema source unavailable")
	}
	var sql string
	ast.Inspect(f, func(n ast.Node) bool {
		v, ok := n.(*ast.ValueSpec)
		if ok && len(v.Names) == 1 && v.Names[0].Name == "schema" && len(v.Values) == 1 {
			if lit, ok := v.Values[0].(*ast.BasicLit); ok {
				sql, _ = strconv.Unquote(lit.Value)
			}
		}
		return true
	})
	sum := sha256.Sum256([]byte(sql))
	if sql == "" || hex.EncodeToString(sum[:]) != d.Ledger.Schema {
		t.Fatal("SQL contract differs")
	}
}
