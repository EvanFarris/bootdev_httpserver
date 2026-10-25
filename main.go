package main

import _ "github.com/lib/pq"
import(
	"log"
	"net/http"
	"sync/atomic"
	"fmt"
	"encoding/json"
	"strings"
	"os"
	"database/sql"
	"github.com/joho/godotenv"
	"github.com/EvanFarris/bootdev_httpserver/internal/database"
	"github.com/EvanFarris/bootdev_httpserver/internal/auth"
	"time"
	"github.com/google/uuid"
	"slices"
)


type apiConfig struct {
	fileserverHits atomic.Int32
	DB *database.Queries
	platform string
	apisecret string
	polkakey string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
	Token	  string	`json:"token"`
	RefreshToken string `json:"refresh_token"`
	IsChirpyRed bool	`json:"is_chirpy_red"`
}

type Chirp struct {
	ID uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body string `json:"body"`
	UserID uuid.UUID `json:"user_id"`
}

type Email struct {
	Email string `json:"email"`
	Password string `json:"password"`
}	

type RefreshToken struct {
	Token string `json:"token"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w,r)
	})
}

func dbUserToStruct(d database.User) User{
	return User{
		ID: d.ID,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
		Email: d.Email,
		IsChirpyRed: d.IsChirpyRed,
	}
}

func dbChirpToStruct(d database.Chirp) Chirp {
	return Chirp{
		ID: d.ID,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
		Body: d.Body,
		UserID: d.UserID,
	}
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
	tok, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, err.Error())
		return
	}

	tokID, err := auth.ValidateJWT(tok, cfg.apisecret)
	if err != nil {
		respondWithError(w, 401, err.Error())
		return
	}

	decoder := json.NewDecoder(r.Body)
	chirp := chirps{}
	err = decoder.Decode(&chirp)
	if err != nil {
		respondWithError(w, 500, "Error Decoding Chirp")
		return
	}

	if len(chirp.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	dbChirp, err := cfg.DB.CreateChirp(r.Context(), database.CreateChirpParams{
		Body: chirp.Body, UserID: tokID})
	if err != nil {
		respondWithError(w,500, err.Error())
		return
	}
	ch := dbChirpToStruct(dbChirp)
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

	ch := dbChirpToStruct(dbChirp)
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
	err = cfg.DB.ResetChirps(r.Context())
	if err != nil {
		respondWithError(w,500, "Error resetting chirps table")
		return
	}
	err = cfg.DB.ResetRefreshTokens(r.Context())
	if err != nil {
		respondWithError(w,500, "Error resetting refresh_token table")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("Reset users table"))
}

func (cfg *apiConfig) addUser(w http.ResponseWriter, r *http.Request) {
	e := Email{}

	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&e)
	if err != nil{
		respondWithError(w, 500, err.Error())
		return
	}

	hashedP, err := auth.HashPassword(e.Password)
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}

	dbUser, err := cfg.DB.CreateUser(r.Context(), database.CreateUserParams{Email: e.Email, HashedPassword: hashedP})
	if err != nil{
		respondWithError(w, 500, err.Error())
		return
	}
	user := dbUserToStruct(dbUser)
	respondWithJSON(w, 201, user)
}

func (cfg *apiConfig) allChirpsHandler(w http.ResponseWriter, r *http.Request) {
	arr, err :=  cfg.DB.GetAllChirps(r.Context())
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}

	var chps []Chirp
	for _, dbChirp := range arr {
		chps = slices.Insert(chps, len(chps), dbChirpToStruct(dbChirp))
	}

	respondWithJSON(w, 200, chps)
}

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {	
	decoder := json.NewDecoder(r.Body)
	e := Email{}
	decoder.Decode(&e)

	dbUser, err := cfg.DB.GetUserByEmail(r.Context(), e.Email)
	if err != nil {
		respondWithError(w, 401, err.Error())
		return
	}

	matched, err := auth.CheckPasswordHash(e.Password, dbUser.HashedPassword)
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}

	if matched == true {
		user := dbUserToStruct(dbUser)
		eis, err := time.ParseDuration("60m")
		if err != nil {
			respondWithError(w, 401, err.Error())
			return
		}
		token, err := auth.MakeJWT(user.ID, cfg.apisecret, eis)
		
		if err != nil {
			respondWithError(w, 401, err.Error())
			return
		}

		rToken := auth.MakeRefreshToken()
		_, err = cfg.DB.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{Token: rToken, UserID: user.ID})
		if err != nil {
			respondWithError(w, 401, err.Error())
			return
		}
		user.Token = token
		user.RefreshToken = rToken
		respondWithJSON(w, 200, user)
	} else {
		respondWithError(w, 401, "Incorrect email or password")
	}
}

func (cfg *apiConfig) refreshHandler(w http.ResponseWriter, r *http.Request){
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if len(token) == 0 {
		respondWithError(w, 401, "No Bearer Token")
		return
	}
	dbTok, err := cfg.DB.GetRefreshToken(r.Context(), token)
	
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		respondWithError(w, 401, err.Error())
		return
	}
	if dbTok.ExpiresAt.Before(time.Now()) || dbTok.RevokedAt.Valid {
		respondWithError(w, 401, "Token expired or revoked")
		return
	}
	eis, err := time.ParseDuration("60m")
	if err != nil {
		respondWithError(w, 401, err.Error())
		return
	}
	newToken, err := auth.MakeJWT(dbTok.UserID, cfg.apisecret, eis)
	if err != nil {
		respondWithError(w, 401, err.Error())
		return
	}
	rt := RefreshToken{Token: newToken}
	respondWithJSON(w, 200, rt)
}

func (cfg *apiConfig) revokeHandler(w http.ResponseWriter, r *http.Request){
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if len(token) == 0 {
		respondWithError(w, 401, "No Bearer Token")
		return
	}

	cfg.DB.RevokeToken(r.Context(), token)

	respondWithError(w, 204, "success")
}

func (cfg *apiConfig) updateHandler(w http.ResponseWriter, r *http.Request){
	tok, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, err.Error())
		return
	}
	uuid, err := auth.ValidateJWT(tok, cfg.apisecret)
	if err != nil {
		respondWithError(w, 401, err.Error())
		return
	}
	decoder := json.NewDecoder(r.Body)
	e := Email{}
	decoder.Decode(&e)
	hashedP, err := auth.HashPassword(e.Password)
	if err != nil {
		respondWithError(w, 401, err.Error())
		return
	}
	row, err := cfg.DB.UpdateUser(r.Context(), database.UpdateUserParams{ID:uuid, Email: e.Email, HashedPassword: hashedP})
	if err != nil {
		respondWithError(w, 401, err.Error())
		return
	}
	respondWithJSON(w, 200, dbUserToStruct(row))
	return 
}

func (cfg *apiConfig) deleteChirpHandler(w http.ResponseWriter, r *http.Request){

	tok, err := auth.GetBearerToken(r.Header)

	if err != nil {
		respondWithError(w, 401, err.Error())
		return
	}
	uid, err := auth.ValidateJWT(tok, cfg.apisecret)
	if err != nil {
		respondWithError(w, 401, err.Error())
		return
	}

	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		respondWithError(w, 404, err.Error())
		return
	}
	
	dbChirp, err := cfg.DB.GetOneChirp(r.Context(), chirpID)
	if err !=  nil {
		respondWithError(w, 404, err.Error())
		return
	}

	if dbChirp.UserID != uid {
		respondWithError(w,403, "uuids don't match")
		return
	}

	err = cfg.DB.DeleteChirp(r.Context(), chirpID)
	if err !=  nil {
		respondWithError(w, 500, err.Error())
		return
	}
	
	respondWithError(w, 204, "Delete successful")
}

func (cfg *apiConfig) chirpyRedWebhook(w http.ResponseWriter, r *http.Request) {
	type UID struct {
		UserID uuid.UUID `json:"user_id"`
	}
	type WebhookEvent struct {
		Event string `json:"event"`
		Data UID `json:"data"`
	}
	whAPIKey, err := auth.GetAPIKey(r.Header)
	if err != nil {
		respondWithError(w, 401, err.Error())
		return
	}

	if cfg.polkakey != whAPIKey {
		respondWithError(w, 401, "Unknown user")
		return
	}
	wh := WebhookEvent{}
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&wh)
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}

	if wh.Event != "user.upgraded" {
		respondWithError(w, 204, "Wrong event")
		return
	}

	err = cfg.DB.UpdateChirpyRed(r.Context(), wh.Data.UserID)
	if err != nil {
		respondWithError(w,404, err.Error())
	} else {
		respondWithError(w,204,"")
	}
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
	cfg.apisecret = os.Getenv("API_SECRET")
	cfg.polkakey = os.Getenv("POLKA_KEY")
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
	httpmux.HandleFunc("POST /api/login", cfg.loginHandler)
	httpmux.HandleFunc("POST /api/refresh", cfg.refreshHandler)
	httpmux.HandleFunc("POST /api/revoke", cfg.revokeHandler)
	httpmux.HandleFunc("PUT /api/users", cfg.updateHandler)
	httpmux.HandleFunc("DELETE /api/chirps/{chirpID}", cfg.deleteChirpHandler)
	httpmux.HandleFunc("POST /api/polka/webhooks", cfg.chirpyRedWebhook)
	svr := &http.Server{
		Addr: ":8080",
		Handler: httpmux,
	}
	log.Fatal(svr.ListenAndServe())
}