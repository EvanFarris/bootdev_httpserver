package main

import(
	"log"
	"net/http"
	"sync/atomic"
	"fmt"
	"encoding/json"
	"strings"
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

func respondWithError(w http.ResponseWriter, code int, msg string){
	type ret struct {
		Error string `json:"error"`
	}
	retErr := ret{
		Error: msg,
	}
	
	data, err := json.Marshal(retErr)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)

}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}){
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func chirpHandler(w http.ResponseWriter, r *http.Request) {
	type chirps struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	chirp := chirps{}
	err := decoder.Decode(&chirp)
	if err != nil {
		respondWithError(w, 500, "Error Decoding Chirp")
		return
	}

	if len(chirp.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	cwords := strings.Split(chirp.Body, " ")

	bannedWords := map[string]bool{
		"kerfuffle": true,
		"sharbert": true,
		"fornax":true,
	}

	for i := 0; i < len(cwords); i++ {
		if _, ok := bannedWords[strings.ToLower(cwords[i])]; ok {
			cwords[i] = "****"
		}
	}
	
	cstr := strings.Join(cwords, " ")

	type ret struct {
		CleanedBody string `json:"cleaned_body"`
	}

	retStr := ret {
		CleanedBody: cstr,
	}


	respondWithJSON(w, 200, retStr)
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
	httpmux.HandleFunc("POST /api/validate_chirp", chirpHandler)
	svr := &http.Server{
		Addr: ":8080",
		Handler: httpmux,
	}
	log.Fatal(svr.ListenAndServe())
}
