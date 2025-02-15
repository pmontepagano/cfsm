package cfsm

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/pmontepagano/cfsm/petrify"
)

const (
	// FreeStateID is the ID used to identify a State unattached to any CFSM.
	FreeStateID = -1
)

// System is a set of CFSMs.
type System struct {
	sync.Mutex
	CFSMs     []*CFSM          // Individual CFSMs in the communicating system.
	cfsmNames map[string]*CFSM // Same CFSMs but ordered by insertion time.
	Comment   string           // Comments on the System.
}

// NewSystem returns a new communicating system
func NewSystem() *System {
	return &System{CFSMs: make([]*CFSM, 0), cfsmNames: make(map[string]*CFSM)}
}

// NewMachine creates a new CFSM in the communicating system and returns it.
// The ID gets assigned a numeric value.
func (s *System) NewMachine() (*CFSM, error) {
	s.Lock()
	defer s.Unlock()
	id := len(s.CFSMs)
	name := strconv.Itoa(id)

	return s.addNewMachine(id, name)
}

// NewMachine creates a new CFSM in the communicating system and returns it.
func (s *System) NewNamedMachine(name string) (*CFSM, error) {
	s.Lock()
	defer s.Unlock()
	id := len(s.CFSMs)

	return s.addNewMachine(id, name)
}

// Helper function for NewMachine and NewNamedMachine. This function assumes the caller has locked the mutex.
func (s *System) addNewMachine(id int, name string) (*CFSM, error) {
	_, ok := s.cfsmNames[name]
	if ok {
		return nil, fmt.Errorf("machine with name %s already exists in the System", name)
	}
	cfsm := &CFSM{ID: id, Name: name}
	s.cfsmNames[name] = cfsm
	s.CFSMs = append(s.CFSMs, cfsm)
	return cfsm, nil
}

// RemoveMachine removes a CFSM with the given id from System.
func (s *System) RemoveMachine(id int) {
	s.Lock()
	defer s.Unlock()
	removed := 0
	for i := range s.CFSMs {
		m := s.CFSMs[i-removed]
		if m.ID == id {
			delete(s.cfsmNames, m.Name)
			s.CFSMs = append(s.CFSMs[:i-removed], s.CFSMs[i-removed+1:]...)
			removed++
		}
	}
	for i, m := range s.CFSMs {
		if intName, err := strconv.Atoi(m.Name); err == nil && intName == m.ID {
			// The name in this CFSM was set from the ID.
			m.Name = strconv.Itoa(i)
		}
		m.ID = i
	}
}

func (s *System) bytesBuffer() *bytes.Buffer {
	var buf bytes.Buffer
	for _, cfsm := range s.CFSMs {
		buf.WriteString(cfsm.String())
	}
	return &buf
}

func (s *System) String() string {
	return s.bytesBuffer().String()
}

func (s *System) Bytes() []byte {
	return s.bytesBuffer().Bytes()
}

// CFSM is a single Communicating Finite State Machine.
type CFSM struct {
	ID      int    // Unique identifier.
	Start   *State // Starting state of the CFSM.
	Comment string // Comments on the CFSM.
	Name    string // Unique name.

	states []*State // States in a CFSM.
}

// NewState creates a new State for this CFSM.
func (m *CFSM) NewState() *State {
	state := &State{ID: len(m.states), edges: make(map[Transition]*State)}
	m.states = append(m.states, state)
	return state
}

// NewFreeState creates a new free State for this CFSM.
func (m *CFSM) NewFreeState() *State {
	state := &State{ID: FreeStateID, edges: make(map[Transition]*State)}
	return state
}

// AddState adds an unattached State to this CFSM.
func (m *CFSM) AddState(s *State) {
	if s.ID == FreeStateID {
		s.ID = len(m.states)
		m.states = append(m.states, s)
	} else {
		log.Fatal("CFSM AddState failed:", ErrStateAlias)
	}
}

// States return states defined in the machine.
func (m *CFSM) States() []*State {
	return m.states
}

// IsEmpty returns true if there are no transitions in the CFSM.
func (m *CFSM) IsEmpty() bool {
	return len(m.states) == 0 || (len(m.states) == 1 && len(m.states[0].edges) == 0)
}

func (m *CFSM) bytesBuffer() *bytes.Buffer {
	var buf bytes.Buffer

	fmap := template.FuncMap{
		"multiline": func(s string) string { return strings.Replace(s, "\n", "\n--", -1) },
	}
	t := template.Must(template.New("petrify").Funcs(fmap).Parse(petrify.Tmpl))
	mach := struct {
		ID      int
		Name    string
		Start   *State
		Comment string
		Edges   []string
	}{
		ID:      m.ID,
		Name:    m.Name,
		Start:   m.Start,
		Comment: m.Comment,
	}
	for _, st := range m.states {
		for tr, st2 := range st.edges {
			mach.Edges = append(mach.Edges, fmt.Sprintf("q%d%d %s q%d%d\n",
				m.ID, st.ID, petrify.Encode(tr.Label()), m.ID, st2.ID))
		}
	}
	err := t.Execute(&buf, mach)
	if err != nil {
		log.Println("Failed to execute template:", err)
	}

	return &buf
}

func (m *CFSM) Bytes() []byte {
	return m.bytesBuffer().Bytes()
}

func (m *CFSM) String() string {
	return m.bytesBuffer().String()
}

// State is a state.
type State struct {
	ID    int    // Unique identifier.
	Label string // Free form text label.

	edges map[Transition]*State
}

// NewState creates a new State independent from any CFSM.
func NewState() *State {
	return &State{ID: -1, edges: make(map[Transition]*State)}
}

// Name of a State is a unique string to identify the State.
func (s *State) Name() string {
	return fmt.Sprintf("q%d", s.ID)
}

// AddTransition adds a transition to the current State.
func (s *State) AddTransition(t Transition) {
	s.edges[t] = t.State()
}

// Transitions returns a list of transitions.
func (s *State) Transitions() []Transition {
	ts := make([]Transition, len(s.edges))
	i := 0
	for t := range s.edges {
		ts[i] = t
		i++
	}
	return ts
}

// Transition is a transition from a State to another State.
type Transition interface {
	Label() string // Label is the marking on the transition.
	State() *State // State after transition.
}

// Send is a send transition (output).
type Send struct {
	to    *CFSM  // Destination CFSM.
	msg   string // Payload message.
	state *State // State after transition.
}

// NewSend returns a new Send transition.
func NewSend(cfsm *CFSM, msg string) *Send {
	return &Send{to: cfsm, msg: msg}
}

// Label for Send is "!"
func (s *Send) Label() string {
	if s.state == nil {
		log.Fatal("Cannot get Label for Send:", ErrStateUndef)
	}
	return fmt.Sprintf("%s ! %s", s.to.Name, s.msg)
}

// State returns the State after transition.
func (s *Send) State() *State {
	return s.state
}

// SetNext sets the next state of the Send transition.
func (s *Send) SetNext(st *State) {
	s.state = st
}

// Recv is a receive transition (input).
type Recv struct {
	from  *CFSM  // Source CFSM.
	msg   string // Payload message expected.
	state *State // State after transition.
}

// NewRecv returns a new Recv transition.
func NewRecv(cfsm *CFSM, msg string) *Recv {
	return &Recv{from: cfsm, msg: msg}
}

// Label for Recv is "?"
func (r *Recv) Label() string {
	if r.state == nil {
		log.Fatal("Cannot get Label for Recv:", ErrStateUndef)
	}
	return fmt.Sprintf("%s ? %s", r.from.Name, r.msg)
}

// State returns the State after transition.
func (r *Recv) State() *State {
	return r.state
}

// SetNext sets the next state of the Recv transition.
func (r *Recv) SetNext(st *State) {
	r.state = st
}
