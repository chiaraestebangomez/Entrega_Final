package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
)

//go:embed static
var content embed.FS

func staticFS() fs.FS {
	sub, err := fs.Sub(content, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

func Assets() http.Handler {
	server := http.StripPrefix("/assets/", http.FileServer(http.FS(staticFS())))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		server.ServeHTTP(w, r)
	})
}

// Config expone al navegador los datos no secretos que la SPA necesita para
// llamar a la API. La API Key es un identificador de cuota (CON-01), no una
// credencial de usuario: el acceso real sigue exigiendo JWT.
func Config(apiKey string) http.Handler {
	payload, _ := json.Marshal(map[string]string{"apiKey": apiKey})
	script := append([]byte("window.TROTAMUNDOS_CONFIG="), payload...)
	script = append(script, ';')
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(script)
	})
}

func Index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := content.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "frontend no disponible", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}
