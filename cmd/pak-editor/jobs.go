package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"solara/internal/pak"
)

// phaseWeights maps each save phase to its [start, end] share of the overall
// progress value.
var phaseWeights = map[string][2]float64{
	pak.PhaseWriting:   {0, 0.45},
	pak.PhaseVerifying: {0.45, 0.85},
	pak.PhaseBackingUp: {0.85, 0.97},
	pak.PhaseSwapping:  {0.97, 1},
}

type jobResult struct {
	Backup         string   `json:"backup"`
	OriginalBackup string   `json:"originalBackup"`
	NewSize        int64    `json:"newSize"`
	Changed        []string `json:"changed"`
	Recompressed   []string `json:"recompressed"`
}

type jobStatus struct {
	State    string     `json:"state"`
	Phase    string     `json:"phase"`
	Progress float64    `json:"progress"`
	Error    string     `json:"error,omitempty"`
	Result   *jobResult `json:"result,omitempty"`
}

type job struct {
	id string
	mu sync.Mutex
	st jobStatus
}

func (j *job) setPhase(phase string, f float64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	w, ok := phaseWeights[phase]
	if !ok {
		return
	}
	j.st.Phase = phase
	if p := w[0] + (w[1]-w[0])*f; p > j.st.Progress {
		j.st.Progress = p
	}
}

func (j *job) finish(res *jobResult, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err != nil {
		j.st.State = "error"
		j.st.Error = err.Error()
		return
	}
	j.st.State = "done"
	j.st.Progress = 1
	j.st.Result = res
}

func (j *job) snapshot() jobStatus {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.st
}

type jobManager struct {
	mu   sync.Mutex
	jobs map[string]*job
}

func newJobManager() *jobManager { return &jobManager{jobs: map[string]*job{}} }

func (m *jobManager) create() *job {
	b := make([]byte, 8)
	rand.Read(b)
	j := &job{id: hex.EncodeToString(b), st: jobStatus{State: "running", Phase: pak.PhaseWriting}}
	m.mu.Lock()
	m.jobs[j.id] = j
	m.mu.Unlock()
	return j
}

func (m *jobManager) get(id string) *job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs[id]
}
