package app

import (
	"testing"

	runtime "github.com/slidebolt/sb-runtime"
)

type resourceRecorder struct{ states []string }

func (r *resourceRecorder) Ready(string)                    { r.states = append(r.states, "ready") }
func (r *resourceRecorder) Degraded(_, _, _ string, _ bool) { r.states = append(r.states, "degraded") }

var _ runtime.ResourceReporter = (*resourceRecorder)(nil)

func TestBridgeStateReportsResourceTransitions(t *testing.T) {
	recorder := &resourceRecorder{}
	p := New()
	p.SetResourceReporter(recorder)
	p.bridgeState = make(chan bool, 2)
	p.handleBridgeState([]byte(`{"state":"online"}`))
	p.handleBridgeState([]byte(`offline`))
	if len(recorder.states) != 2 || recorder.states[0] != "ready" || recorder.states[1] != "degraded" {
		t.Fatalf("states = %v", recorder.states)
	}
	if !<-p.bridgeState || <-p.bridgeState {
		t.Fatal("bridge state signals did not preserve online/offline")
	}
}
