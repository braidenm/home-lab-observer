package workloadprojection

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestContainerSectionReusesExactExistingProjection(t *testing.T) {
	raw, docker, id := inputs()
	want, err := Encode(raw, docker, id, Consent{Processes: true, Containers: true})
	if err != nil {
		t.Fatal("existing projection refused")
	}
	s, err := ProjectContainers(docker)
	if err != nil {
		t.Fatal("section projection refused")
	}
	b, err := s.Bytes()
	if err != nil {
		t.Fatal("section encoding refused")
	}
	parsed, err := ParseContainers(b)
	if err != nil {
		t.Fatal("canonical section refused")
	}
	got, err := EncodeProcessesAndContainers(raw, id, parsed)
	if err != nil || !bytes.Equal(want, got) {
		t.Fatal("separate section changed existing wire")
	}
	at, ok := parsed.ObservedAt()
	if !ok || !at.Equal(*docker.ObservedAt) {
		t.Fatal("observation time changed")
	}
	docker.Items[0].Name = "changed"
	*docker.Items[0].MemoryBytes = 99
	b[0] = '!'
	again, err := s.Bytes()
	if err != nil || bytes.Contains(again, []byte("changed")) || again[0] != '{' {
		t.Fatal("section aliases caller data")
	}
	if _, err := ParseContainers(b); err == nil {
		t.Fatal("modified bytes accepted")
	}
}

func TestContainerSectionPreservesItsOlderObservationTime(t *testing.T) {
	raw, docker, id := inputs()
	s, err := ProjectContainers(docker)
	if err != nil {
		t.Fatal("section refused")
	}
	raw.ObservedAt = raw.ObservedAt.Add(30 * time.Second)
	b, err := EncodeProcessesAndContainers(raw, id, s)
	if err != nil {
		t.Fatal("composition refused")
	}
	var d document
	if json.Unmarshal(b, &d) != nil || d.Containers.ObservedAt == nil || *d.Containers.ObservedAt != docker.ObservedAt.Format(time.RFC3339Nano) || d.At == *d.Containers.ObservedAt {
		t.Fatal("cached rows were retimestamped")
	}
}

func TestContainerSectionParserRejectsPrivacyMalformedAndForeignShapes(t *testing.T) {
	_, docker, _ := inputs()
	s, err := ProjectContainers(docker)
	if err != nil {
		t.Fatal("section refused")
	}
	b, _ := s.Bytes()
	for index, bad := range [][]byte{
		nil, bytes.Repeat([]byte("x"), MaxSectionBytes+1), append([]byte(" "), b...),
		bytes.Replace(b, []byte(`"quality":`), []byte(`"unknown":"synthetic-private","quality":`), 1),
		bytes.Replace(b, []byte(`"quality":`), []byte(`"quality":null,"quality":`), 1),
		bytes.Replace(b, []byte("sample-service"), []byte("github"+"_pat_"+strings.Repeat("x", 24)), 1),
		bytes.Replace(b, []byte("ctr_0000000000000002"), []byte("ctr_0000000000000001"), 1),
		bytes.Replace(b, []byte(`"metrics_state":"PARTIAL"`), []byte(`"metrics_state":"AVAILABLE"`), 1),
		bytes.Replace(b, []byte(`"cpu_measurement_basis":"ENGINE_HOST_CAPACITY"`), []byte(`"cpu_measurement_basis":"LIFETIME_SINGLE_CORE"`), 1),
		[]byte(strings.Replace(string(b), `"sample-service"`, `"sample\u0001service"`, 1)),
	} {
		if _, err := ParseContainers(bad); err == nil {
			t.Fatalf("invalid section case %d accepted", index)
		}
	}
}

func TestUnavailableContainerSectionIsNotHealthyEmpty(t *testing.T) {
	raw, _, id := inputs()
	s := UnavailableContainers()
	if _, ok := s.ObservedAt(); ok {
		t.Fatal("unavailable section fabricated time")
	}
	b, err := EncodeProcessesAndContainers(raw, id, s)
	if err != nil {
		t.Fatal("unavailable section refused")
	}
	var d document
	if json.Unmarshal(b, &d) != nil || !d.Policy.Processes || !d.Policy.Containers || d.Containers.Quality.State != "UNAVAILABLE" || d.Containers.Total != nil || len(d.Containers.Items) != 0 || len(d.Processes.Items) != 1 {
		t.Fatal("unavailability changed consent or fabricated a count")
	}
	if _, err := (ContainerSection{}).Bytes(); err == nil {
		t.Fatal("zero section accepted")
	}
	if _, err := EncodeProcessesAndContainers(raw, id, ContainerSection{}); err == nil {
		t.Fatal("zero section composed")
	}
}
