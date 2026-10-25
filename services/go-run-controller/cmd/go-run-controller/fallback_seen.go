package main

import "sync"

// maxFallbackSeen acota las corridas ya avisadas por el respaldo RUN_ARTIFACT_REF (U11-T03 F-04): el mapa no
// crece sin límite en un controlador de vida larga.
const maxFallbackSeen = 10000

// seenBounded recuerda IDs ya vistos con un tope: al llenarse descarta el más antiguo (FIFO).
type seenBounded struct {
	mu    sync.Mutex
	max   int
	set   map[string]struct{}
	order []string
}

func newSeenBounded(max int) *seenBounded {
	return &seenBounded{max: max, set: make(map[string]struct{})}
}

// addIfNew devuelve true solo la primera vez que ve id (mientras siga recordado).
func (s *seenBounded) addIfNew(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.set[id]; ok {
		return false
	}
	if len(s.order) >= s.max {
		old := s.order[0]
		s.order = s.order[1:]
		delete(s.set, old)
	}
	s.set[id] = struct{}{}
	s.order = append(s.order, id)
	return true
}

// len cuenta los IDs recordados (nunca supera max).
func (s *seenBounded) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.set)
}
