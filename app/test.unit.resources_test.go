package app

import (
	"testing"

	runtime "github.com/slidebolt/sb-runtime"
)

func TestUnconfiguredPluginStartsDisabled(t *testing.T) {
	t.Setenv("Z2M_MQTT_BROKER", "")

	p := New()
	if _, err := p.OnStart(nil); err != nil {
		t.Fatalf("unconfigured plugin must remain disabled and ready: %v", err)
	}
	if p.msg != nil || p.store != nil || p.mqtt != nil {
		t.Fatal("disabled plugin initialized runtime dependencies")
	}
}

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
