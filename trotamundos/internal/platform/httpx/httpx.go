package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"trotamundos/internal/platform/idgen"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

func Decode(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		// Body vacío: se mantiene el comportamiento histórico (equivale a
		// "sin datos"), útil para clientes que sólo envían headers.
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	if dec.More() {
		return errors.New("request body must contain a single JSON value")
	}
	return nil
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = idgen.UUID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func RequestIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r)
			log.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"bytes", sw.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", RequestIDFrom(r.Context()),
				"remote", ClientIP(r),
			)
		})
	}
}

// recoverWriter registra si el handler ya escribió (explícita o
// implícitamente) la cabecera de respuesta.
type recoverWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *recoverWriter) WriteHeader(status int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *recoverWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

func (w *recoverWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *recoverWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &recoverWriter{ResponseWriter: w}
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered", "panic", rec, "stack", string(debug.Stack()))
					// Si ya se envió la cabecera no tiene sentido intentar
					// escribir un 500: sólo se loguea el panic.
					if !rw.wroteHeader {
						Error(rw, http.StatusInternalServerError, "internal_error", "unexpected error")
					}
				}
			}()
			next.ServeHTTP(rw, r)
		})
	}
}

// SafeChain arma la cadena de middlewares garantizando que Recover sea la
// capa más externa. Es compatible con Chain: se puede seguir pasando un
// httpx.Recover en la lista (el interno queda redundante pero inofensivo).
func SafeChain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	return Chain(Chain(h, mws...), Recover(nil))
}

func addVaryOrigin(w http.ResponseWriter) {
	for _, v := range w.Header().Values("Vary") {
		for _, name := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(name), "Origin") {
				return
			}
		}
	}
	w.Header().Add("Vary", "Origin")
}

func CORS(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if allowedOrigin == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				addVaryOrigin(w)
			} else if origin != "" && origin == allowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				addVaryOrigin(w)
			}
			w.Header().Set("Access-Control-Max-Age", "600")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, X-Mock-Key, X-Webhook-Key, X-Request-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// ClientIP sólo confía en X-Forwarded-For cuando RemoteAddr pertenece a un
// proxy de confianza. Configurar TRUSTED_PROXIES con una lista separada por
// comas de IPs o CIDRs (por ejemplo "10.0.0.0/8, 192.168.1.10"). Sin esa
// variable X-Forwarded-For se ignora por completo.
func ClientIP(r *http.Request) string {
	remote := remoteHost(r)
	ip := net.ParseIP(stripZone(remote))
	if ip == nil || !isTrustedProxy(ip) {
		return remote
	}

	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	// Se recorre de derecha a izquierda: el valor más a la derecha es el
	// asignado por el proxy más cercano y es el único difícil de falsificar.
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := net.ParseIP(stripZone(strings.TrimSpace(parts[i])))
		if candidate == nil {
			continue
		}
		if !isTrustedProxy(candidate) {
			return candidate.String()
		}
	}
	return remote
}

func remoteHost(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func stripZone(host string) string {
	if i := strings.IndexByte(host, '%'); i >= 0 {
		return host[:i]
	}
	return host
}

var (
	trustedMu         sync.RWMutex
	trustedNetworks   []*net.IPNet
	trustedAddresses  []net.IP
	proxiesConfigured bool
)

// ConfigureTrustedProxies fija la lista desde config.Load(). Si no se llama,
// ClientIP mantiene el fallback histórico a la variable TRUSTED_PROXIES.
func ConfigureTrustedProxies(entries []string) {
	networks, addresses := parseTrustedProxies(entries)
	trustedMu.Lock()
	defer trustedMu.Unlock()
	trustedNetworks = networks
	trustedAddresses = addresses
	proxiesConfigured = true
}

// ResetTrustedProxies sólo existe para tests: vuelve al fallback por entorno.
func ResetTrustedProxies() {
	trustedMu.Lock()
	defer trustedMu.Unlock()
	trustedNetworks = nil
	trustedAddresses = nil
	proxiesConfigured = false
}

func isTrustedProxy(ip net.IP) bool {
	trustedMu.RLock()
	configured := proxiesConfigured
	networks := append([]*net.IPNet(nil), trustedNetworks...)
	addresses := append([]net.IP(nil), trustedAddresses...)
	trustedMu.RUnlock()

	if configured {
		return ipInTrustedList(ip, networks, addresses)
	}
	raw := os.Getenv("TRUSTED_PROXIES")
	if strings.TrimSpace(raw) == "" {
		return false
	}
	networks, addresses = parseTrustedProxies(strings.Split(raw, ","))
	return ipInTrustedList(ip, networks, addresses)
}

func parseTrustedProxies(entries []string) ([]*net.IPNet, []net.IP) {
	var networks []*net.IPNet
	var addresses []net.IP
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, network, err := net.ParseCIDR(entry); err == nil {
				networks = append(networks, network)
			}
			continue
		}
		if candidate := net.ParseIP(entry); candidate != nil {
			addresses = append(addresses, candidate)
		}
	}
	return networks, addresses
}

func ipInTrustedList(ip net.IP, networks []*net.IPNet, addresses []net.IP) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	for _, candidate := range addresses {
		if candidate.Equal(ip) {
			return true
		}
	}
	return false
}
