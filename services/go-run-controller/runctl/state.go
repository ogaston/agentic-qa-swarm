// Package runctl es el dominio de go-run-controller: la máquina de estados de una corrida,
// los puertos hacia el mundo exterior y el controlador que pregunta a U4 antes de cada
// transición. No importa HTTP, archivos ni Kubernetes: todo entra por puertos.
package runctl

// State es un valor de components.schemas.Run.state del OpenAPI.
type State string

// Estados de una corrida (enum exacto del OpenAPI).
const (
	Confirmed  State = "confirmed"
	WarmReady  State = "warm_ready"
	Deploying  State = "deploying"
	Inferring  State = "inferring"
	Rehearsing State = "rehearsing"
	Running    State = "running"
	Resetting  State = "resetting"
	Reporting  State = "reporting"
	Done       State = "done"
	Failed     State = "failed"
)

// AllStates lista los estados del enum.
var AllStates = []State{Confirmed, WarmReady, Deploying, Inferring, Rehearsing, Running, Resetting, Reporting, Done, Failed}

// chain es la cadena lineal; resettable son los estados desde los que se puede ir a resetting
// además de la cadena (fin o fallo de corrida: reset verificado antes de cerrar).
var chain = map[State]State{
	Confirmed: WarmReady, WarmReady: Deploying, Deploying: Inferring, Inferring: Rehearsing,
	Rehearsing: Running, Running: Resetting, Resetting: Reporting, Reporting: Done,
}

// Valid indica si s pertenece al enum.
func (s State) Valid() bool {
	for _, v := range AllStates {
		if s == v {
			return true
		}
	}
	return false
}

// Terminal indica si s es done o failed.
func (s State) Terminal() bool { return s == Done || s == Failed }

// Resettable indica si desde s se va a resetting al fallar o terminar la corrida.
func (s State) Resettable() bool {
	return s == Deploying || s == Inferring || s == Rehearsing || s == Running
}

// Legal indica si from -> to es una transición legal (19 en total: 8 de la cadena,
// 8 hacia failed desde cualquier estado no terminal y 3 hacia resetting).
func Legal(from, to State) bool {
	if !from.Valid() || !to.Valid() || from.Terminal() {
		return false
	}
	if to == Failed {
		return true
	}
	if chain[from] == to {
		return true
	}
	return to == Resetting && from.Resettable()
}
