package output

import (
	"bytes"
	"testing"

	"github.com/elft/KubeSqueeze/internal/selection"
)

func TestWriteStableShape(t *testing.T) {
	replicas, zero := int32(3), int32(0)
	value := Result{Operation: "squeeze", Cluster: Cluster{Context: "dev", Server: "https://example.invalid"}, Discovered: 2, Included: 1, Ignored: []IgnoredResource{{Namespace: "team", Kind: selection.KindCronJob, Name: "keep", Categories: []selection.Category{selection.CategoryAnnotation}}}, Mutated: []MutatedResource{{Namespace: "team", Kind: selection.KindDeployment, Name: "api", Previous: State{Replicas: &replicas}, Current: State{Replicas: &zero}, Status: "updated"}}}
	var got bytes.Buffer
	if err := Write(&got, value); err != nil {
		t.Fatal(err)
	}
	want := `{"operation":"squeeze","cluster":{"context":"dev","server":"https://example.invalid"},"discovered":2,"included":1,"ignored":[{"namespace":"team","kind":"cronjob","name":"keep","categories":["annotation"]}],"mutated":[{"namespace":"team","kind":"deployment","name":"api","previous":{"replicas":3},"current":{"replicas":0},"status":"updated"}]}` + "\n"
	if got.String() != want {
		t.Fatalf("output = %q, want %q", got.String(), want)
	}
}
