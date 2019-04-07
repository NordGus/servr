// 8) Crear un contenedor de Docker donde esté ejecutándose el
// servidor anterior y se puedan utilizar todas las APIs implementadas
// anteriormente desde fuera del contenedor.

package main

import (
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	dbname   = "./servr.db"
	driver   = "sqlite3"
	certFile = "./certs/app.crt" // Certificados propios para pruebas
	certKey  = "./certs/app.key" // Certificados propios para pruebas
)

func init() {
	err := createSumTableIfDoesntExist()
	if err != nil {
		log.Println(err)
		os.Exit(7)
	}
}

func main() {
	addr := fmt.Sprintf(":%v", 8080)

	r := http.NewServeMux()

	r.HandleFunc("/api/v1/hello", RequestLogger(HelloChameleon))
	r.HandleFunc("/api/v1/sum", RequestLogger(Sum))
	r.HandleFunc("/api/v1/sumdb", RequestLogger(SumDB))
	r.HandleFunc("/api/v1/reset", RequestLogger(Reset))

	s := NewServer(addr, r)

	err := s.ListenAndServeTLS(certFile, certKey)

	if err != nil {
		log.Println(err)
		os.Exit(7)
	}
}

// NewServer crea y configura el servidor para servir la applicaion a la web
func NewServer(serverAddress string, mux *http.ServeMux) *http.Server {
	// Blog de Cloudflare con la explicación del porque estas configuraciones:
	// https://blog.cloudflare.com/exposing-go-on-the-internet/
	tlsConfig := &tls.Config{
		PreferServerCipherSuites: true,
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.X25519,
		},
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	return &http.Server{
		Addr:         serverAddress,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig:    tlsConfig,
	}
}

// HelloChameleon escucha a la URL "/api/v1/hello" y retorna "Hello Chamalleon" como respuesta.
func HelloChameleon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("Hello Chameleon"))
	if err != nil {
		http.Error(w, fmt.Sprintf("Opps!, algo salió mal. Error: %v", err), http.StatusInternalServerError)
		return
	}
}

// Sum escucha a la URL "/api/v1/sum" esperando dos numeros enteros, "a" y "b", como entrada
// y devuelve la suma de los mismos como salida.
func Sum(w http.ResponseWriter, r *http.Request) {
	a, b, err := retrieveNumbers(r.URL.Query())
	if err != nil {
		http.Error(w, fmt.Sprint("Opps algo salió mal. Error: ", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	result := strconv.Itoa(a + b)
	_, err = w.Write([]byte(result))
	if err != nil {
		http.Error(w, fmt.Sprintf("Opps!, algo salió mal. Error: %v", err), http.StatusInternalServerError)
		return
	}
}

// SumDB escucha a la URL "/api/v1/sumdb" esperando dos numeros enteros, "a" y "b", como entrada,
// calcula la suma de los mismos, guarda ambos números y el resultado de la suma en la base de datos
// y retorna el número de resultados en la base de datos.
func SumDB(w http.ResponseWriter, r *http.Request) {
	a, b, err := retrieveNumbers(r.URL.Query())
	if err != nil {
		http.Error(w, fmt.Sprint("Opps algo salió mal. Error: ", err), http.StatusInternalServerError)
		return
	}
	total, err := registerSumInDB(a, b)
	if err != nil {
		http.Error(w, fmt.Sprint("Opps algo salió mal. Error: ", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	result := strconv.Itoa(total)
	_, err = w.Write([]byte(result))
	if err != nil {
		http.Error(w, fmt.Sprintf("Opps!, algo salió mal. Error: %v", err), http.StatusInternalServerError)
		return
	}
}

// Reset escucha a la URL "/api/v1/reset" borra todos los registros de sumas y sus resultados
// de la base de datos y no devuelve contenido.
func Reset(w http.ResponseWriter, r *http.Request) {
	db, err := sql.Open(driver, dbname)
	if err != nil {
		http.Error(w, fmt.Sprint("Opps algo salió mal. Error: ", err), http.StatusInternalServerError)
		return
	}
	defer db.Close()
	result, err := db.Exec("DELETE FROM sums;")
	if err != nil {
		http.Error(w, fmt.Sprint("Opps algo salió mal. Error: ", err), http.StatusInternalServerError)
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, fmt.Sprint("Opps algo salió mal. Error: ", err), http.StatusInternalServerError)
		return
	}
	log.Println("Número de operaciones borradas:", affected)
	w.WriteHeader(http.StatusNoContent)
}

// RequestLogger logs incoming request
func RequestLogger(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		log.Printf("%v | [%v] %v - %v \n", r.Proto, r.RemoteAddr, r.Method, r.RequestURI)
		f(w, r)
		log.Printf("%v | [%v] %v - %v: request processed in %s\n", r.Proto, r.RemoteAddr, r.Method, r.RequestURI, time.Now().Sub(started))
	}
}

// retrieveNumbers procesa los parametros de la url y devuelve los numeros a sumar o un error en caso de que algo falle
func retrieveNumbers(values map[string][]string) (int, int, error) {
	var numbers []int
	if len(values) != 2 {
		return 0, 0, errors.New("no se recibieron los dos números enteros en la URL")
	}
	for key, value := range values {
		number, err := strconv.Atoi(value[0])
		if err != nil {
			return 0, 0, fmt.Errorf("\"%v\" no es un valor válido para \"%v\"", value[0], key)
		}
		numbers = append(numbers, number)
	}
	return numbers[0], numbers[1], nil
}

// CreateSumTableIfDoesntExist es una función para que en el momento de la inicialización del programa
// que revisa si exista la base de datos y la tabla necesaria dentro de ella
func createSumTableIfDoesntExist() error {
	db, err := sql.Open(driver, dbname)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS sums (id INTEGER PRIMARY KEY AUTOINCREMENT, first_number INTEGER NOT NULL, second_number INTEGER NOT NULL, total INTEGER NOT NULL);`)
	if err != nil {
		return err
	}
	return nil
}

// registerSumInDB toma dos números enteros, "a" y "b", calcula la suma de los mismos,
// guarda ambos números y el resultado de la suma en la base de datos,
// calcula el número de resultados en la base de datos y lo retorna o un error en caso de que algo falle
func registerSumInDB(a, b int) (int, error) {
	db, err := sql.Open(driver, dbname)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	insert, err := db.Prepare("INSERT INTO sums(first_number, second_number, total) VALUES (?, ?, ?);")
	if err != nil {
		return 0, err
	}
	defer insert.Close()
	_, err = insert.Exec(a, b, (a + b))
	if err != nil {
		return 0, err
	}
	count, err := db.Query("SELECT COUNT(total) FROM sums;")
	if err != nil {
		return 0, err
	}
	defer count.Close()
	var total int
	for count.Next() {
		err = count.Scan(&total)
		if err != nil {
			return 0, err
		}
		break
	}
	db.Close()
	insert.Close()
	count.Close()
	return total, nil
}
