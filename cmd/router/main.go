package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
)

const (
	defaultPort      = 8080
	defaultAdminPort = 7676
)

var routes = make(map[string]string)

func main() {
	port := flag.Int("port", defaultPort, "main port used for access")
	adminPort := flag.Int("admin-port", defaultAdminPort, "admin port used for configuration and monitoring")

	go func() {
		adminServer := http.NewServeMux()
		adminServer.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				listRoutes(rw, r)
			case http.MethodPost:
				createRoute(rw, r)
			case http.MethodDelete:
				deleteRoute(rw, r)
			default:
				log.Printf("not found: %s", r.URL.Path)
				http.NotFound(rw, r)

				return
			}
		})
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *adminPort), adminServer))
	}()

	server := http.NewServeMux()
	server.HandleFunc("/", applyRoute)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), server))
}

func applyRoute(rw http.ResponseWriter, r *http.Request) {
	schema := "http"
	if r.TLS != nil {
		schema = "https"
	}

	host := routes[r.Host]
	if host == "" {
		log.Printf("route not found: %s", r.Host)
		http.NotFound(rw, r)

		return
	}

	url := fmt.Sprintf("%s://%s%s", schema, host, r.URL.Path)

	req, err := http.NewRequest(r.Method, url, r.Body)
	if err != nil {
		log.Printf("error creating request: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("error getting content at %q: %v", url, err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("error reading response body: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	for name, values := range resp.Header {
		for _, v := range values {
			rw.Header().Add(name, v)
		}
	}

	rw.WriteHeader(resp.StatusCode)
	fmt.Fprint(rw, string(b))
}

func listRoutes(rw http.ResponseWriter, r *http.Request) {
	b, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		log.Printf("error marshaling routes: %v", err)
		http.Error(rw, "", http.StatusInternalServerError)

		return
	}

	rw.Header().Add("Content-Type", http.DetectContentType(b))
	fmt.Fprint(rw, string(b))
}

type createRouteDTO struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func createRoute(rw http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading route from request body: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	var route createRouteDTO

	err = json.Unmarshal(b, &route)
	if err != nil {
		log.Printf("error unmarshaling route from request body: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	routes[route.From] = route.To
}

type deleteRouteDTO struct {
	From string `json:"from"`
}

func deleteRoute(rw http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading route from request body: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	var route deleteRouteDTO

	err = json.Unmarshal(b, &route)
	if err != nil {
		log.Printf("error unmarshaling route from request body: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	delete(routes, route.From)
}
