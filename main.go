package main

import(
	"log"
	"net/http"
	"sync/atomic"
	"fmt"
)


type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w,r)
	})
	
	
	
}

func (cfg *apiConfig) writeHits(rw http.ResponseWriter, _*http.Request) {
	hstr:= fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", cfg.fileserverHits.Load())
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.WriteHeader(200)
	rw.Write([]byte(hstr))
}

func (cfg *apiConfig) resetHits(rw http.ResponseWriter, _*http.Request) {
	cfg.fileserverHits.Store(0)
	hstr := fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())

	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rw.WriteHeader(200)
	rw.Write([]byte(hstr))
}

func main() {
	cfg := apiConfig{}
	cfg.fileserverHits.Store(0)
	h1 := func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		rw.WriteHeader(200)
		rw.Write([]byte("OK"))
	}

	httpmux := http.NewServeMux()
	httpmux.Handle("/app", http.StripPrefix("/app", cfg.middlewareMetricsInc(http.FileServer(http.Dir(".")))))
	httpmux.Handle("/app/", http.StripPrefix("/app/", cfg.middlewareMetricsInc(http.FileServer(http.Dir(".")))))
	httpmux.HandleFunc("GET /api/healthz", h1)
	httpmux.HandleFunc("GET /admin/metrics", cfg.writeHits)
	httpmux.HandleFunc("POST /admin/reset", cfg.resetHits)
	svr := &http.Server{
		Addr: ":8080",
		Handler: httpmux,
	}
	log.Fatal(svr.ListenAndServe())
}
