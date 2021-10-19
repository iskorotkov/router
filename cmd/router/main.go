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
	"sync"

	"github.com/iskorotkov/router/internal/models"
	"github.com/iskorotkov/router/internal/routing"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	defaultPort      = 8080
	defaultAdminPort = 7676
)

//nolint:gochecknoglobals
var (
	db          *gorm.DB
	routes      routing.Cache
	syncWorkers sync.WaitGroup

	indexTemplate    = template.Must(template.ParseFiles("./static/html/index.html"))
	notFoundTemplate = template.Must(template.ParseFiles("./static/html/404.html"))

	ErrValidation = fmt.Errorf("validation failed")
)

func main() {
	var err error

	db, err = gorm.Open(sqlite.Open("var/db.sqlite"))
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}

	if err := db.AutoMigrate(
		&models.Route{}, //nolint:exhaustivestruct
	); err != nil {
		log.Fatalf("error running migrations: %v", err)
	}

	populateRoutes(db)

	port := flag.Int("port", defaultPort, "main port used for access")
	adminPort := flag.Int("admin-port", defaultAdminPort, "admin port used for configuration and monitoring")

	defer func() {
		log.Printf("waiting for all sync workers to finish their work")

		syncWorkers.Wait()
	}()

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

	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), server); err != nil {
		log.Printf("error in server: %v", err)

		return
	}
}

func populateRoutes(db *gorm.DB) {
	var storedRoutes []models.Route

	if err := db.Order("id DESC").Find(&storedRoutes).Error; err != nil {
		log.Fatalf("error reading stored routes from db: %v", err)
	}

	routes = routing.New()

	for _, route := range storedRoutes {
		routes.Set(route.From, routing.RouteInfo{
			To:   route.To,
			Type: route.Type,
		})
	}
}

func applyRoute(rw http.ResponseWriter, r *http.Request) {
	schema := "http"
	if r.TLS != nil {
		schema = "https"
	}

	origins, err := normalizeAddress(fmt.Sprintf("%s://%s", schema, r.RemoteAddr))
	if err != nil {
		log.Printf("error parsing remote address: %v", err)
		http.Error(rw, "", http.StatusInternalServerError)

		return
	}

	for _, origin := range origins {
		info, ok := routes.Get(origin)
		if !ok {
			continue
		}

		otherURL := fmt.Sprintf("%s://%s%s", schema, info.To, r.URL.Path)

		switch info.Type {
		case models.RouteTypeRedirect:
			http.Redirect(rw, r, otherURL, http.StatusTemporaryRedirect)
		case models.RouteTypeProxy:
			proxyRequest(rw, r, otherURL)
		default:
			log.Printf("unknown route type %q", info.Type)
			http.Error(rw, "", http.StatusInternalServerError)

			return
		}

		return
	}

	log.Printf("no route configured for host %q", r.RemoteAddr)
	rw.WriteHeader(http.StatusBadGateway)
}

func normalizeAddress(address string) ([]string, error) {
	parsed, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("error normalizing address %q: %w", address, err)
	}

	switch parsed.Hostname() {
	case "::1", "127.0.0.1", "localhost":
		return []string{"[::1]", "127.0.0.1", "localhost"}, nil
	default:
		return []string{address}, nil
	}
}

func proxyRequest(rw http.ResponseWriter, r *http.Request, otherURL string) {
	req, err := http.NewRequestWithContext(context.Background(), r.Method, otherURL, r.Body)
	if err != nil {
		log.Printf("error creating request: %v", err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	req.Header = r.Header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("error getting content at %q: %v", otherURL, err)
		http.Error(rw, "", http.StatusBadRequest)

		return
	}

	defer resp.Body.Close()

	for name, values := range resp.Header {
		for _, v := range values {
			rw.Header().Add(name, v)
		}
	}

	rw.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(rw, resp.Body); err != nil {
		log.Printf("error copying request body: %v", err)
		http.Error(rw, "", http.StatusInternalServerError)

		return
	}
}

func listRoutes(rw http.ResponseWriter, _ *http.Request) {
	b, err := json.MarshalIndent(routes.GetAll(), "", "  ")
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
	Type models.RouteType
}

func (c *createRouteDTO) Validate() error {
	c.From = strings.TrimSpace(c.From)
	c.To = strings.TrimSpace(c.To)

	if c.From == "" || c.To == "" {
		return fmt.Errorf("one of the fields of %v is empty: %w", c, ErrValidation)
	}

	if c.Type != models.RouteTypeProxy && c.Type != models.RouteTypeRedirect {
		return fmt.Errorf("route type of %v is invalid: %w", c, ErrValidation)
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

	routes.Set(route.From, routing.RouteInfo{
		To:   route.To,
		Type: route.Type,
	})

	syncWorkers.Add(1)

	go func() {
		defer syncWorkers.Done()

		if err := db.Save(&models.Route{
			Model: gorm.Model{}, //nolint:exhaustivestruct
			From:  route.From,
			To:    route.To,
			Type:  route.Type,
		}).Error; err != nil {
			log.Printf("error saving route to db: %v", err)

			return
		}

		log.Printf("route saved to db")
	}()
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

	routes.Remove(route.From)

	syncWorkers.Add(1)

	go func() {
		defer syncWorkers.Done()

		if err := db.Where("`from` = ?", route.From).Delete(&models.Route{}).Error; err != nil { //nolint:exhaustivestruct
			log.Printf("error deleting route from db: %v", err)

			return
		}

		log.Printf("route deleted from db")
	}()
}

func showDashboard(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		show404(rw, r)

		return
	}

	if err := indexTemplate.Execute(rw, struct {
		Routes map[string]routing.RouteInfo
	}{
		routes.GetAll(),
	}); err != nil {
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
