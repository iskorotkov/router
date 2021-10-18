package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

const (
	defaultPort      = 8080
	defaultAdminPort = 7676
)

//nolint:gochecknoglobals
var (
	routes           = make(map[string]string)
	indexTemplate    = template.Must(template.ParseFiles("./static/html/index.html"))
	notFoundTemplate = template.Must(template.ParseFiles("./static/html/404.html"))

	ErrValidation = fmt.Errorf("validation failed")
)

func main() {
	port := flag.Int("port", defaultPort, "main port used for access")
	adminPort := flag.Int("admin-port", defaultAdminPort, "admin port used for configuration and monitoring")

	go func() {
		adminServer := http.NewServeMux()
		adminServer.HandleFunc("/api/v1/routes", func(rw http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				listRoutes(rw, r)
			case http.MethodPost:
				createRoute(rw, r)
			case http.MethodDelete:
				deleteRoute(rw, r)
			default:
				api404(rw, r)

				return
			}
		})
		adminServer.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("./static/css"))))
		adminServer.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("./static/js"))))
		adminServer.HandleFunc("/", showDashboard)
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

	referer := r.Header.Get("Referer")

	refererURL, err := url.Parse(referer)
	if err != nil {
		log.Printf("error parsing referer %q: %v", referer, err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	host := routes[refererURL.Host]
	if host == "" {
		api404(rw, r)

		return
	}

	otherURL := fmt.Sprintf("%s://%s%s", schema, host, r.URL.Path)

	req, err := http.NewRequestWithContext(context.Background(), r.Method, otherURL, r.Body)
	if err != nil {
		log.Printf("error creating request: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("error getting content at %q: %v", otherURL, err)
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
	_, _ = fmt.Fprint(rw, string(b))
}

func listRoutes(rw http.ResponseWriter, _ *http.Request) {
	b, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		log.Printf("error marshaling routes: %v", err)
		http.Error(rw, "", http.StatusInternalServerError)

		return
	}

	rw.Header().Add("Content-Type", http.DetectContentType(b))
	_, _ = fmt.Fprint(rw, string(b))
}

type createRouteDTO struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (c *createRouteDTO) Validate() error {
	c.From = strings.TrimSpace(c.From)
	c.To = strings.TrimSpace(c.To)

	if c.From == "" || c.To == "" {
		return fmt.Errorf("one of the fields of %v is empty: %w", c, ErrValidation)
	}

	return nil
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

	if err := route.Validate(); err != nil {
		log.Printf("error validating dto: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	routes[route.From] = route.To
}

type deleteRouteDTO struct {
	From string `json:"from"`
}

func (d *deleteRouteDTO) Validate() error {
	d.From = strings.TrimSpace(d.From)

	if d.From == "" {
		return fmt.Errorf("one of the fields of %v is empty: %w", d, ErrValidation)
	}

	return nil
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

	if err := route.Validate(); err != nil {
		log.Printf("error validating dto: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	delete(routes, route.From)
}

func showDashboard(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		show404(rw, r)

		return
	}

	if err := indexTemplate.Execute(rw, struct {
		Routes map[string]string
	}{routes}); err != nil {
		log.Printf("error executing template: %v", err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}
}

func api404(rw http.ResponseWriter, r *http.Request) {
	log.Printf("not found: %s", r.URL.Path)
	http.NotFound(rw, r)
}

func show404(rw http.ResponseWriter, r *http.Request) {
	log.Printf("not found: %s", r.URL.Path)

	if err := notFoundTemplate.Execute(rw, nil); err != nil {
		log.Printf("error executing template: %v", err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}
}
