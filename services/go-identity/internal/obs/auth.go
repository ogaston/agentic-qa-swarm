package obs

import "github.com/prometheus/client_golang/prometheus"

// Etiquetas acotadas de las métricas de identidad.
var (
	// LoginResults son los valores de aqs_auth_login_total{result}.
	LoginResults = []string{"success", "invalid_credentials", "mfa_required", "locked", "bad_request"}
	// AuthzReasons son los valores de aqs_authz_denied_total{reason}.
	AuthzReasons = []string{"unauthorized", "forbidden"}
)

// Auth son las métricas de seguridad de go-identity. Todas las series con contador
// arrancan en 0. Un *Auth nil no hace nada.
type Auth struct {
	logins      *prometheus.CounterVec
	lockouts    prometheus.Counter
	authzDenied *prometheus.CounterVec
	escalations *prometheus.CounterVec
}

// NewAuth registra las métricas. activeSessions se evalúa en cada scrape (aqs_auth_active_sessions).
// escalationEndpoints son los PATRONES de ruta que pueden contar como intento de escalada.
func NewAuth(reg prometheus.Registerer, activeSessions func() float64, escalationEndpoints []string) *Auth {
	a := &Auth{
		logins: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_auth_login_total",
			Help: "Resultados de POST /auth/login."}, []string{"result"}),
		lockouts: prometheus.NewCounter(prometheus.CounterOpts{Name: "aqs_auth_lockouts_total",
			Help: "Bloqueos anti fuerza bruta impuestos."}),
		authzDenied: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_authz_denied_total",
			Help: "Accesos denegados por falta de autenticación o de rol."}, []string{"reason"}),
		escalations: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aqs_privilege_escalation_attempts_total",
			Help: "Intentos de un user autenticado de usar rutas de admin o sesiones ajenas, por patrón de ruta."}, []string{"endpoint"}),
	}
	reg.MustRegister(a.logins, a.lockouts, a.authzDenied, a.escalations,
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "aqs_auth_active_sessions",
			Help: "Sesiones vigentes en memoria."}, activeSessions))
	for _, r := range LoginResults {
		a.logins.WithLabelValues(r)
	}
	for _, r := range AuthzReasons {
		a.authzDenied.WithLabelValues(r)
	}
	for _, e := range escalationEndpoints {
		a.escalations.WithLabelValues(e)
	}
	return a
}

// Login cuenta un resultado de login (uno de LoginResults).
func (a *Auth) Login(result string) {
	if a != nil {
		a.logins.WithLabelValues(result).Inc()
	}
}

// Lockout cuenta un bloqueo impuesto.
func (a *Auth) Lockout() {
	if a != nil {
		a.lockouts.Inc()
	}
}

// AuthzDenied cuenta un acceso denegado (uno de AuthzReasons).
func (a *Auth) AuthzDenied(reason string) {
	if a != nil {
		a.authzDenied.WithLabelValues(reason).Inc()
	}
}

// Escalation cuenta un intento de escalada; endpoint es el patrón de la ruta.
func (a *Auth) Escalation(endpoint string) {
	if a != nil {
		a.escalations.WithLabelValues(endpoint).Inc()
	}
}
