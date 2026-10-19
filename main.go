package main

import _ "github.com/lib/pq"
import(
	"log"
	"net/http"
	"sync/atomic"
	"fmt"
	"encoding/json"
	//"strings"
	"os"
	"database/sql"
	"github.com/joho/godotenv"
	"github.com/EvanFarris/bootdev_httpserver/internal/database"
	"time"
	"github.com/google/uuid"
	"slices"
)


type apiConfig struct {
	fileserverHits atomic.Int32
	DB *database.Queries
	platform string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body string `json:"body"`
	UserID uuid.UUID `json:"user_id"`
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

func (cfg *apiConfig) chirpHandler(w http.ResponseWriter, r *http.Request) {
	type chirps struct {
		Body string `json:"body"`
		UserID uuid.UUID `json:"user_id"`
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
	/*
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
	*/
	//fmt.Printf("Body: %s\nUserID: %s\n", chirp.Body, chirp.UserID)

	dbChirp, err := cfg.DB.CreateChirp(r.Context(), database.CreateChirpParams{
		Body: chirp.Body, UserID: chirp.UserID})
	if err != nil {
		respondWithError(w,500, err.Error())
		return
	}
	ch := Chirp {
		ID: dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body: dbChirp.Body,
		UserID: dbChirp.UserID,
	}
	respondWithJSON(w, 201, ch)
}

func (cfg *apiConfig) getChirpHandler(w http.ResponseWriter, r *http.Request) {
	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}
	dbChirp, err := cfg.DB.GetOneChirp(r.Context(), chirpID)

	if err !=  nil {
		respondWithError(w, 404, err.Error())
		return
	}

	ch := Chirp {
		ID: dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body: dbChirp.Body,
		UserID: dbChirp.UserID,
	}
	respondWithJSON(w, 200, ch)
}

func (cfg *apiConfig) writeHits(rw http.ResponseWriter, _*http.Request) {
	hstr:= fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", cfg.fileserverHits.Load())
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.WriteHeader(200)
	rw.Write([]byte(hstr))
}

func (cfg *apiConfig) resetHits(w http.ResponseWriter, _*http.Request) {
	cfg.fileserverHits.Store(0)
	hstr := fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte(hstr))
}

func (cfg *apiConfig) resetUsers(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(403)
		w.Write([]byte("403 Forbidden"))
		return
	}

	err := cfg.DB.ResetUsers(r.Context())
	if err != nil {
		respondWithError(w,500, "Error resetting Users table")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("Reset users table"))
}

func (cfg *apiConfig) addUser(w http.ResponseWriter, r *http.Request) {
	type email struct {
		Email string `json:"email"`
	}

	e := email{}

	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&e)
	if err != nil{
		respondWithError(w, 500, "Error decoding email")
		return
	}

	user, err := cfg.DB.CreateUser(r.Context(), e.Email)
	if err != nil{
		respondWithError(w, 500, "Error creating user")
		return
	}
	usr := User{
		ID: user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email: user.Email,
	}
	respondWithJSON(w, 201, usr)
}

func (cfg *apiConfig) allChirpsHandler(w http.ResponseWriter, r *http.Request) {
	arr, err :=  cfg.DB.GetAllChirps(r.Context())
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}

	var chps []Chirp
	for _, dbChirp := range arr {
		chps = slices.Insert(chps, len(chps), Chirp{
			ID: dbChirp.ID,
			CreatedAt: dbChirp.CreatedAt,
			UpdatedAt: dbChirp.UpdatedAt,
			Body: dbChirp.Body,
			UserID: dbChirp.UserID,
		})
	}

	respondWithJSON(w, 200, chps)
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Printf("%s\n", err)
		return
	}
	dbQueries := database.New(db)
	if dbQueries != nil {
		fmt.Printf("database made\n")
	}

	cfg := apiConfig{}
	cfg.fileserverHits.Store(0)
	cfg.DB = dbQueries
	cfg.platform = os.Getenv("PLATFORM")

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
	httpmux.HandleFunc("POST /admin/reset", cfg.resetUsers)
	httpmux.HandleFunc("POST /api/chirps", cfg.chirpHandler)
	httpmux.HandleFunc("POST /api/users", cfg.addUser)
	httpmux.HandleFunc("GET /api/chirps", cfg.allChirpsHandler)
	httpmux.HandleFunc("GET /api/chirps/{chirpID}", cfg.getChirpHandler)

	svr := &http.Server{
		Addr: ":8080",
		Handler: httpmux,
	}
	log.Fatal(svr.ListenAndServe())
}
