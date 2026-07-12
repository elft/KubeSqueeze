package output

import (
	"encoding/json"
	"io"

	"github.com/elft/KubeSqueeze/internal/selection"
)

type Cluster struct {
	Context string `json:"context"`
	Server  string `json:"server"`
}

type State struct {
	Replicas *int32 `json:"replicas,omitempty"`
	Suspend  *bool  `json:"suspend,omitempty"`
}

type IgnoredResource struct {
	Namespace  string               `json:"namespace"`
	Kind       selection.Kind       `json:"kind"`
	Name       string               `json:"name"`
	Categories []selection.Category `json:"categories"`
}

type MutatedResource struct {
	Namespace   string            `json:"namespace"`
	Kind        selection.Kind    `json:"kind"`
	Name        string            `json:"name"`
	Previous    State             `json:"previous"`
	Current     State             `json:"current"`
	Status      string            `json:"status"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type Result struct {
	Operation  string            `json:"operation"`
	DryRun     bool              `json:"dryRun,omitempty"`
	Cluster    Cluster           `json:"cluster"`
	Discovered int               `json:"discovered"`
	Included   int               `json:"included"`
	Ignored    []IgnoredResource `json:"ignored"`
	Mutated    []MutatedResource `json:"mutated"`
}

type Error struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Cluster *Cluster `json:"cluster,omitempty"`
}

func Write(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
